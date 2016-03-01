package bulk

import (
	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	"github.com/pivotal-golang/lager"
)

type TaskDiffer interface {
	Diff(lager.Logger, <-chan []cc_messages.CCTaskState, map[string]*models.Task, <-chan struct{})
	TasksToFail() <-chan []cc_messages.CCTaskState
	TasksToCancel() <-chan []string
}

type taskDiffer struct {
	tasksToFail   chan []cc_messages.CCTaskState
	tasksToCancel chan []string
}

func NewTaskDiffer() TaskDiffer {
	return &taskDiffer{
		tasksToFail:   make(chan []cc_messages.CCTaskState, 1),
		tasksToCancel: make(chan []string, 1),
	}
}

func (t *taskDiffer) Diff(logger lager.Logger, ccTasks <-chan []cc_messages.CCTaskState, bbsTasks map[string]*models.Task, cancelCh <-chan struct{}) {
	logger = logger.Session("task_diff")

	tasksToCancel := cloneBbsTasks(bbsTasks)

	go func() {
		defer func() {
			close(t.tasksToFail)
		}()

		for {
			select {
			case <-cancelCh:
				return

			case batchCCTasks, open := <-ccTasks:
				if !open {
					guids := filterTasksToCancel(tasksToCancel)
					if len(guids) > 0 {
						t.tasksToCancel <- guids
					}

					return
				}

				batchTasksToFail := []cc_messages.CCTaskState{}
				for _, ccTask := range batchCCTasks {

					_, exists := bbsTasks[ccTask.TaskGuid]

					if exists {
						if ccTask.State != cc_messages.TaskStateCanceling {
							delete(tasksToCancel, ccTask.TaskGuid)
						}
					} else {
						if ccTask.State == cc_messages.TaskStateRunning {
							batchTasksToFail = append(batchTasksToFail, ccTask)

							logger.Info("found-unkown-to-diego-task", lager.Data{
								"guid": ccTask.TaskGuid,
							})
						}
					}
				}

				if len(batchTasksToFail) > 0 {
					t.tasksToFail <- batchTasksToFail
				}
			}
		}
	}()
}

func (t *taskDiffer) TasksToFail() <-chan []cc_messages.CCTaskState {
	return t.tasksToFail
}

func (t *taskDiffer) TasksToCancel() <-chan []string {
	return t.tasksToCancel
}

func cloneBbsTasks(bbsTasks map[string]*models.Task) map[string]*models.Task {
	clone := map[string]*models.Task{}
	for k, v := range bbsTasks {
		clone[k] = v
	}
	return clone
}

func filterTasksToCancel(tasksToCancel map[string]*models.Task) []string {
	guids := make([]string, 0, len(tasksToCancel))
	for _, bbsTask := range tasksToCancel {
		if bbsTask.State == models.Task_Completed || bbsTask.State == models.Task_Resolving {
			continue
		}
		guids = append(guids, bbsTask.TaskGuid)
	}
	return guids
}
