// This file was generated by counterfeiter
package fakes

import (
	"sync"

	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/cloudfoundry-incubator/nsync/recipebuilder"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
)

type FakeRecipeBuilder struct {
	BuildStub        func(*cc_messages.DesireAppRequestFromCC) (*models.DesiredLRP, error)
	buildMutex       sync.RWMutex
	buildArgsForCall []struct {
		arg1 *cc_messages.DesireAppRequestFromCC
	}
	buildReturns struct {
		result1 *models.DesiredLRP
		result2 error
	}
	BuildTaskStub        func(*cc_messages.TaskRequestFromCC) (*models.TaskDefinition, error)
	buildTaskMutex       sync.RWMutex
	buildTaskArgsForCall []struct {
		arg1 *cc_messages.TaskRequestFromCC
	}
	buildTaskReturns struct {
		result1 *models.TaskDefinition
		result2 error
	}
	ExtractExposedPortsStub        func(*cc_messages.DesireAppRequestFromCC) ([]uint32, error)
	extractExposedPortsMutex       sync.RWMutex
	extractExposedPortsArgsForCall []struct {
		arg1 *cc_messages.DesireAppRequestFromCC
	}
	extractExposedPortsReturns struct {
		result1 []uint32
		result2 error
	}
}

func (fake *FakeRecipeBuilder) Build(arg1 *cc_messages.DesireAppRequestFromCC) (*models.DesiredLRP, error) {
	fake.buildMutex.Lock()
	fake.buildArgsForCall = append(fake.buildArgsForCall, struct {
		arg1 *cc_messages.DesireAppRequestFromCC
	}{arg1})
	fake.buildMutex.Unlock()
	if fake.BuildStub != nil {
		return fake.BuildStub(arg1)
	} else {
		return fake.buildReturns.result1, fake.buildReturns.result2
	}
}

func (fake *FakeRecipeBuilder) BuildCallCount() int {
	fake.buildMutex.RLock()
	defer fake.buildMutex.RUnlock()
	return len(fake.buildArgsForCall)
}

func (fake *FakeRecipeBuilder) BuildArgsForCall(i int) *cc_messages.DesireAppRequestFromCC {
	fake.buildMutex.RLock()
	defer fake.buildMutex.RUnlock()
	return fake.buildArgsForCall[i].arg1
}

func (fake *FakeRecipeBuilder) BuildReturns(result1 *models.DesiredLRP, result2 error) {
	fake.BuildStub = nil
	fake.buildReturns = struct {
		result1 *models.DesiredLRP
		result2 error
	}{result1, result2}
}

func (fake *FakeRecipeBuilder) BuildTask(arg1 *cc_messages.TaskRequestFromCC) (*models.TaskDefinition, error) {
	fake.buildTaskMutex.Lock()
	fake.buildTaskArgsForCall = append(fake.buildTaskArgsForCall, struct {
		arg1 *cc_messages.TaskRequestFromCC
	}{arg1})
	fake.buildTaskMutex.Unlock()
	if fake.BuildTaskStub != nil {
		return fake.BuildTaskStub(arg1)
	} else {
		return fake.buildTaskReturns.result1, fake.buildTaskReturns.result2
	}
}

func (fake *FakeRecipeBuilder) BuildTaskCallCount() int {
	fake.buildTaskMutex.RLock()
	defer fake.buildTaskMutex.RUnlock()
	return len(fake.buildTaskArgsForCall)
}

func (fake *FakeRecipeBuilder) BuildTaskArgsForCall(i int) *cc_messages.TaskRequestFromCC {
	fake.buildTaskMutex.RLock()
	defer fake.buildTaskMutex.RUnlock()
	return fake.buildTaskArgsForCall[i].arg1
}

func (fake *FakeRecipeBuilder) BuildTaskReturns(result1 *models.TaskDefinition, result2 error) {
	fake.BuildTaskStub = nil
	fake.buildTaskReturns = struct {
		result1 *models.TaskDefinition
		result2 error
	}{result1, result2}
}

func (fake *FakeRecipeBuilder) ExtractExposedPorts(arg1 *cc_messages.DesireAppRequestFromCC) ([]uint32, error) {
	fake.extractExposedPortsMutex.Lock()
	fake.extractExposedPortsArgsForCall = append(fake.extractExposedPortsArgsForCall, struct {
		arg1 *cc_messages.DesireAppRequestFromCC
	}{arg1})
	fake.extractExposedPortsMutex.Unlock()
	if fake.ExtractExposedPortsStub != nil {
		return fake.ExtractExposedPortsStub(arg1)
	} else {
		return fake.extractExposedPortsReturns.result1, fake.extractExposedPortsReturns.result2
	}
}

func (fake *FakeRecipeBuilder) ExtractExposedPortsCallCount() int {
	fake.extractExposedPortsMutex.RLock()
	defer fake.extractExposedPortsMutex.RUnlock()
	return len(fake.extractExposedPortsArgsForCall)
}

func (fake *FakeRecipeBuilder) ExtractExposedPortsArgsForCall(i int) *cc_messages.DesireAppRequestFromCC {
	fake.extractExposedPortsMutex.RLock()
	defer fake.extractExposedPortsMutex.RUnlock()
	return fake.extractExposedPortsArgsForCall[i].arg1
}

func (fake *FakeRecipeBuilder) ExtractExposedPortsReturns(result1 []uint32, result2 error) {
	fake.ExtractExposedPortsStub = nil
	fake.extractExposedPortsReturns = struct {
		result1 []uint32
		result2 error
	}{result1, result2}
}

var _ recipebuilder.RecipeBuilder = new(FakeRecipeBuilder)
