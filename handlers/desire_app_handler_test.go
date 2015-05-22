package handlers_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/cloudfoundry-incubator/nsync/bulk/fakes"
	"github.com/cloudfoundry-incubator/nsync/handlers"
	"github.com/cloudfoundry-incubator/nsync/recipebuilder"
	"github.com/cloudfoundry-incubator/receptor"
	"github.com/cloudfoundry-incubator/receptor/fake_receptor"
	"github.com/cloudfoundry-incubator/route-emitter/cfroutes"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/cloudfoundry/dropsonde/metric_sender/fake"
	"github.com/cloudfoundry/dropsonde/metrics"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("DesireAppHandler", func() {
	var (
		logger           *lagertest.TestLogger
		fakeReceptor     *fake_receptor.FakeClient
		builder          *fakes.FakeRecipeBuilder
		desireAppRequest cc_messages.DesireAppRequestFromCC
		metricSender     *fake.FakeMetricSender

		request          *http.Request
		responseRecorder *httptest.ResponseRecorder
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test")
		fakeReceptor = new(fake_receptor.FakeClient)
		builder = new(fakes.FakeRecipeBuilder)

		desireAppRequest = cc_messages.DesireAppRequestFromCC{
			ProcessGuid:  "some-guid",
			DropletUri:   "http://the-droplet.uri.com",
			Stack:        "some-stack",
			StartCommand: "the-start-command",
			Environment: cc_messages.Environment{
				{Name: "foo", Value: "bar"},
				{Name: "VCAP_APPLICATION", Value: "{\"application_name\":\"my-app\"}"},
			},
			MemoryMB:        128,
			DiskMB:          512,
			FileDescriptors: 32,
			NumInstances:    2,
			Routes:          []string{"route1", "route2"},
			LogGuid:         "some-log-guid",
			ETag:            "last-modified-etag",
		}

		metricSender = fake.NewFakeMetricSender()
		metrics.Initialize(metricSender)

		responseRecorder = httptest.NewRecorder()

		var err error
		request, err = http.NewRequest("POST", "", nil)
		Expect(err).NotTo(HaveOccurred())
		request.Form = url.Values{
			":process_guid": []string{"some-guid"},
		}
	})

	JustBeforeEach(func() {
		if request.Body == nil {
			jsonBytes, err := json.Marshal(&desireAppRequest)
			Expect(err).NotTo(HaveOccurred())
			reader := bytes.NewReader(jsonBytes)

			request.Body = ioutil.NopCloser(reader)
		}

		handler := handlers.NewDesireAppHandler(logger, fakeReceptor, builder)
		handler.DesireApp(responseRecorder, request)
	})

	Context("when the desired LRP does not exist", func() {
		var newlyDesiredLRP receptor.DesiredLRPCreateRequest

		BeforeEach(func() {
			newlyDesiredLRP = receptor.DesiredLRPCreateRequest{
				ProcessGuid: "new-process-guid",
				Instances:   1,
				RootFS:      models.PreloadedRootFS("stack-2"),
				Action: &models.RunAction{
					Path: "ls",
				},
				Annotation: "last-modified-etag",
			}

			fakeReceptor.GetDesiredLRPReturns(receptor.DesiredLRPResponse{}, receptor.Error{
				Type:    receptor.DesiredLRPNotFound,
				Message: "Desired LRP with guid 'new-process-guid' not found",
			})
			builder.BuildReturns(&newlyDesiredLRP, nil)
		})

		It("creates the desired LRP", func() {
			Expect(fakeReceptor.CreateDesiredLRPCallCount()).To(Equal(1))

			Expect(fakeReceptor.GetDesiredLRPCallCount()).To(Equal(1))
			Expect(fakeReceptor.CreateDesiredLRPArgsForCall(0)).To(Equal(newlyDesiredLRP))

			Expect(builder.BuildArgsForCall(0)).To(Equal(&desireAppRequest))
		})

		It("responds with 202 Accepted", func() {
			Expect(responseRecorder.Code).To(Equal(http.StatusAccepted))
		})

		It("increments the desired LRPs counter", func() {
			Expect(metricSender.GetCounter("LRPsDesired")).To(Equal(uint64(1)))
		})

		Context("when the receptor fails", func() {
			BeforeEach(func() {
				fakeReceptor.CreateDesiredLRPReturns(errors.New("oh no"))
			})

			It("responds with a ServiceUnavailabe error", func() {
				Expect(responseRecorder.Code).To(Equal(http.StatusServiceUnavailable))
			})
		})

		Context("when the receptor fails with a Conflict error", func() {
			BeforeEach(func() {
				fakeReceptor.CreateDesiredLRPStub = func(_ receptor.DesiredLRPCreateRequest) error {
					fakeReceptor.GetDesiredLRPReturns(receptor.DesiredLRPResponse{
						ProcessGuid: "some-guid",
						Routes:      receptor.RoutingInfo{},
					}, nil)
					return receptor.Error{Type: receptor.DesiredLRPAlreadyExists}
				}
			})

			It("retries", func() {
				Expect(fakeReceptor.CreateDesiredLRPCallCount()).To(Equal(1))
				Expect(fakeReceptor.UpdateDesiredLRPCallCount()).To(Equal(1))
			})

			It("suceeds if the second try is sucessful", func() {
				Expect(responseRecorder.Code).To(Equal(http.StatusAccepted))
			})

			Context("when updating the desired LRP fails with a conflict error", func() {
				BeforeEach(func() {
					fakeReceptor.UpdateDesiredLRPReturns(receptor.Error{Type: receptor.ResourceConflict})
				})

				It("fails with a 409 Conflict if the second try is unsuccessful", func() {
					Expect(responseRecorder.Code).To(Equal(http.StatusConflict))
				})
			})
		})

		Context("when building the recipe fails to build", func() {
			BeforeEach(func() {
				builder.BuildReturns(nil, recipebuilder.ErrAppSourceMissing)
			})

			It("logs an error", func() {
				Eventually(logger.TestSink.Buffer).Should(gbytes.Say("failed-to-build-recipe"))
				Eventually(logger.TestSink.Buffer).Should(gbytes.Say(recipebuilder.ErrAppSourceMissing.Message))
			})

			It("does not desire the LRP", func() {
				Consistently(fakeReceptor.DeleteDesiredLRPCallCount).Should(Equal(0))
			})

			It("responds with 400 Bad Request", func() {
				Expect(responseRecorder.Code).To(Equal(http.StatusBadRequest))
			})
		})
	})

	Context("when desired LRP already exists", func() {
		var opaqueRoutingMessage json.RawMessage

		BeforeEach(func() {
			cfRoute := cfroutes.CFRoute{
				Hostnames: []string{"route1"},
				Port:      8080,
			}
			cfRoutePayload, err := json.Marshal(cfRoute)
			Expect(err).NotTo(HaveOccurred())

			cfRouteMessage := json.RawMessage(cfRoutePayload)
			opaqueRoutingMessage = json.RawMessage([]byte(`{"some": "value"}`))

			fakeReceptor.GetDesiredLRPReturns(receptor.DesiredLRPResponse{
				ProcessGuid: "some-guid",
				Routes: receptor.RoutingInfo{
					cfroutes.CF_ROUTER:        &cfRouteMessage,
					"some-other-routing-data": &opaqueRoutingMessage,
				},
			}, nil)
		})

		It("checks to see if LRP already exists", func() {
			Eventually(fakeReceptor.GetDesiredLRPCallCount).Should(Equal(1))
		})

		It("updates the LRP without stepping on opaque routing data", func() {
			Eventually(fakeReceptor.UpdateDesiredLRPCallCount).Should(Equal(1))

			processGuid, updateRequest := fakeReceptor.UpdateDesiredLRPArgsForCall(0)
			Expect(processGuid).To(Equal("some-guid"))
			Expect(*updateRequest.Instances).To(Equal(2))
			Expect(*updateRequest.Annotation).To(Equal("last-modified-etag"))

			expectedRoutePayload, err := json.Marshal(cfroutes.CFRoutes{
				{Hostnames: []string{"route1", "route2"}, Port: 8080},
			})
			Expect(err).NotTo(HaveOccurred())

			expectedCfRouteMessage := json.RawMessage(expectedRoutePayload)
			Expect(updateRequest.Routes).To(Equal(receptor.RoutingInfo{
				cfroutes.CF_ROUTER:        &expectedCfRouteMessage,
				"some-other-routing-data": &opaqueRoutingMessage,
			}))

		})

		It("responds with 202 Accepted", func() {
			Expect(responseRecorder.Code).To(Equal(http.StatusAccepted))
		})

		Context("when the receptor fails", func() {
			BeforeEach(func() {
				fakeReceptor.UpdateDesiredLRPReturns(errors.New("oh no"))
			})

			It("responds with a ServiceUnavailabe error", func() {
				Expect(responseRecorder.Code).To(Equal(http.StatusServiceUnavailable))
			})
		})

		Context("when the receptor fails with a conflict", func() {
			BeforeEach(func() {
				fakeReceptor.UpdateDesiredLRPReturns(receptor.Error{Type: receptor.ResourceConflict})
			})

			It("responds with a Conflict error", func() {
				Expect(responseRecorder.Code).To(Equal(http.StatusConflict))
			})
		})
	})

	Context("when an invalid desire app message is received", func() {
		BeforeEach(func() {
			reader := bytes.NewBufferString("not valid json")
			request.Body = ioutil.NopCloser(reader)
		})

		It("does not call the receptor", func() {
			Expect(fakeReceptor.KillActualLRPByProcessGuidAndIndexCallCount()).To(BeZero())
		})

		It("responds with 400 Bad Request", func() {
			Expect(responseRecorder.Code).To(Equal(http.StatusBadRequest))
		})

		It("logs an error", func() {
			Eventually(logger.TestSink.Buffer).Should(gbytes.Say("parse-desired-app-request-failed"))
		})

		It("does not touch the LRP", func() {
			Expect(fakeReceptor.CreateDesiredLRPCallCount()).To(Equal(0))
			Expect(fakeReceptor.UpdateDesiredLRPCallCount()).To(Equal(0))
			Expect(fakeReceptor.DeleteDesiredLRPCallCount()).To(Equal(0))
		})

	})

	Context("when the process guids do not match", func() {
		BeforeEach(func() {
			request.Form.Set(":process_guid", "another-guid")
		})

		It("does not call the receptor", func() {
			Expect(fakeReceptor.KillActualLRPByProcessGuidAndIndexCallCount()).To(BeZero())
		})

		It("responds with 400 Bad Request", func() {
			Expect(responseRecorder.Code).To(Equal(http.StatusBadRequest))
		})

		It("logs an error", func() {
			Eventually(logger.TestSink.Buffer).Should(gbytes.Say("desire-app.process-guid-mismatch"))
		})

		It("does not touch the LRP", func() {
			Expect(fakeReceptor.CreateDesiredLRPCallCount()).To(Equal(0))
			Expect(fakeReceptor.UpdateDesiredLRPCallCount()).To(Equal(0))
			Expect(fakeReceptor.DeleteDesiredLRPCallCount()).To(Equal(0))
		})
	})
})
