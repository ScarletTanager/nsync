package handlers_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/cloudfoundry-incubator/nsync/handlers"
	"github.com/cloudfoundry-incubator/receptor"
	"github.com/cloudfoundry-incubator/receptor/fake_receptor"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("KillIndexHandler", func() {
	var (
		logger       *lagertest.TestLogger
		fakeReceptor *fake_receptor.FakeClient

		request          *http.Request
		responseRecorder *httptest.ResponseRecorder
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test")
		fakeReceptor = new(fake_receptor.FakeClient)

		responseRecorder = httptest.NewRecorder()

		var err error
		request, err = http.NewRequest("POST", "", nil)
		Expect(err).NotTo(HaveOccurred())
		request.Form = url.Values{
			":process_guid": []string{"process-guid-0"},
			":index":        []string{"1"},
		}
	})

	JustBeforeEach(func() {
		killHandler := handlers.NewKillIndexHandler(logger, fakeReceptor)
		killHandler.KillIndex(responseRecorder, request)
	})

	It("invokes the receptor to kill", func() {
		Expect(fakeReceptor.KillActualLRPByProcessGuidAndIndexCallCount()).To(Equal(1))

		processGuid, stopIndex := fakeReceptor.KillActualLRPByProcessGuidAndIndexArgsForCall(0)
		Expect(processGuid).To(Equal("process-guid-0"))
		Expect(stopIndex).To(Equal(1))
	})

	It("responds with 202 Accepted", func() {
		Expect(responseRecorder.Code).To(Equal(http.StatusAccepted))
	})

	Context("when the receptor fails", func() {
		BeforeEach(func() {
			fakeReceptor.KillActualLRPByProcessGuidAndIndexReturns(errors.New("oh no"))
		})

		It("responds with a ServiceUnavailabe error", func() {
			Expect(responseRecorder.Code).To(Equal(http.StatusServiceUnavailable))
		})
	})

	Context("when the receptor cannot find the guid", func() {
		BeforeEach(func() {
			fakeReceptor.KillActualLRPByProcessGuidAndIndexReturns(receptor.Error{Type: receptor.ActualLRPIndexNotFound})
		})

		It("responds with a NotFound error", func() {
			Expect(responseRecorder.Code).To(Equal(http.StatusNotFound))
		})
	})

	Context("when the process guid is missing", func() {
		BeforeEach(func() {
			request.Form.Del(":process_guid")
		})

		It("does not call the receptor", func() {
			Expect(fakeReceptor.KillActualLRPByProcessGuidAndIndexCallCount()).To(BeZero())
		})

		It("responds with 400 Bad Request", func() {
			Expect(responseRecorder.Code).To(Equal(http.StatusBadRequest))
		})
	})

	Context("when the index is missing", func() {
		BeforeEach(func() {
			request.Form.Del(":index")
		})

		It("does not call the receptor", func() {
			Expect(fakeReceptor.KillActualLRPByProcessGuidAndIndexCallCount()).To(BeZero())
		})

		It("responds with 400 Bad Request", func() {
			Expect(responseRecorder.Code).To(Equal(http.StatusBadRequest))
		})
	})

	Context("when the index is not a number", func() {
		BeforeEach(func() {
			request.Form.Set(":index", "not-a-number")
		})

		It("does not call the receptor", func() {
			Expect(fakeReceptor.KillActualLRPByProcessGuidAndIndexCallCount()).To(BeZero())
		})

		It("responds with 400 Bad Request", func() {
			Expect(responseRecorder.Code).To(Equal(http.StatusBadRequest))
		})
	})

	Context("when the index is out of range", func() {
		BeforeEach(func() {
			request.Form.Set(":index", "5")
			fakeReceptor.KillActualLRPByProcessGuidAndIndexReturns(receptor.Error{
				Type: receptor.ActualLRPIndexNotFound,
			})
		})

		It("does call the receptor", func() {
			Expect(fakeReceptor.KillActualLRPByProcessGuidAndIndexCallCount()).To(Equal(1))
		})

		It("responds with 404 Not Found", func() {
			Expect(responseRecorder.Code).To(Equal(http.StatusNotFound))
		})
	})

})
