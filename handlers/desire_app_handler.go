package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/cloudfoundry-incubator/nsync/recipebuilder"
	"github.com/cloudfoundry-incubator/receptor"
	"github.com/cloudfoundry-incubator/route-emitter/cfroutes"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	"github.com/cloudfoundry-incubator/runtime-schema/metric"
	"github.com/pivotal-golang/lager"
)

const (
	desiredLRPCounter = metric.Counter("LRPsDesired")
)

//go:generate counterfeiter -o fakes/fake_recipe_builder.go . RecipeBuilder
type RecipeBuilder interface {
	Build(*cc_messages.DesireAppRequestFromCC) (*receptor.DesiredLRPCreateRequest, error)
}

type DesireAppHandler struct {
	recipeBuilder  RecipeBuilder
	receptorClient receptor.Client
	logger         lager.Logger
}

func NewDesireAppHandler(logger lager.Logger, receptorClient receptor.Client, builder RecipeBuilder) DesireAppHandler {
	return DesireAppHandler{
		recipeBuilder:  builder,
		receptorClient: receptorClient,
		logger:         logger,
	}
}

func (h *DesireAppHandler) DesireApp(resp http.ResponseWriter, req *http.Request) {
	processGuid := req.FormValue(":process_guid")
	logger := h.logger.Session("desire-app", lager.Data{
		"process_guid": processGuid,
	})

	desiredApp := cc_messages.DesireAppRequestFromCC{}
	err := json.NewDecoder(req.Body).Decode(&desiredApp)
	if err != nil {
		logger.Error("parse-desired-app-request-failed", err)
		resp.WriteHeader(http.StatusBadRequest)
		return
	}

	if processGuid != desiredApp.ProcessGuid {
		logger.Error("process-guid-mismatch", err, lager.Data{"body-process-guid": desiredApp.ProcessGuid})
		resp.WriteHeader(http.StatusBadRequest)
		return
	}

	existingLRP, err := h.getDesiredLRP(processGuid)
	if err != nil {
		logger.Error("unexpected-error-from-get-desired-lrp", err)
		resp.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	desiredLRPCounter.Increment()

	if existingLRP != nil {
		err = h.updateDesiredApp(logger, resp, existingLRP, desiredApp)
		if err != nil {
			resp.WriteHeader(http.StatusServiceUnavailable)
			return
		}
	} else {
		desiredLRP, err := h.recipeBuilder.Build(&desiredApp)
		if err != nil {
			logger.Error("failed-to-build-recipe", err)
			resp.WriteHeader(http.StatusInternalServerError)
			return
		}

		err = h.receptorClient.CreateDesiredLRP(*desiredLRP)
		if err != nil {
			logger.Error("failed-to-create", err)
			resp.WriteHeader(http.StatusServiceUnavailable)
			return
		}
	}

	resp.WriteHeader(http.StatusAccepted)
}

func (h *DesireAppHandler) getDesiredLRP(processGuid string) (*receptor.DesiredLRPResponse, error) {
	lrp, err := h.receptorClient.GetDesiredLRP(processGuid)
	if err == nil {
		return &lrp, nil
	}

	if rerr, ok := err.(receptor.Error); ok {
		if rerr.Type == receptor.DesiredLRPNotFound {
			return nil, nil
		}
	}

	return nil, err
}

func (h *DesireAppHandler) updateDesiredApp(
	logger lager.Logger,
	resp http.ResponseWriter,
	existingLRP *receptor.DesiredLRPResponse,
	desireAppMessage cc_messages.DesireAppRequestFromCC,
) error {
	routes := existingLRP.Routes

	cfRoutesJson, err := json.Marshal(cfroutes.CFRoutes{
		{Hostnames: desireAppMessage.Routes, Port: recipebuilder.DefaultPort},
	})
	if err != nil {
		logger.Error("failed-to-marshal-routes", err)
		return err
	}

	cfRoutesMessage := json.RawMessage(cfRoutesJson)
	routes[cfroutes.CF_ROUTER] = &cfRoutesMessage

	updateRequest := receptor.DesiredLRPUpdateRequest{
		Annotation: &desireAppMessage.ETag,
		Instances:  &desireAppMessage.NumInstances,
		Routes:     routes,
	}

	err = h.receptorClient.UpdateDesiredLRP(desireAppMessage.ProcessGuid, updateRequest)
	if err != nil {
		logger.Error("failed-to-update-lrp", err)
		return err
	}

	return nil
}

func (h *DesireAppHandler) deleteDesiredApp(logger lager.Logger, processGuid string) error {
	err := h.receptorClient.DeleteDesiredLRP(processGuid)
	if err == nil {
		return nil
	}

	if rerr, ok := err.(receptor.Error); ok {
		if rerr.Type == receptor.DesiredLRPNotFound {
			logger.Info("lrp-already-deleted")
			return nil
		}
	}

	logger.Error("failed-to-remove", err)
	return err
}
