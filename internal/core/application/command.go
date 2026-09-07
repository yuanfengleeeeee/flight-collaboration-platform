package application

import (
	"context"
	"fmt"

	coreexception "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/exception"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/flighttask"
	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
)

// HandleCommand is the Worker command router. Foundation probes remain
// available for infrastructure tests; business commands enter the approved
// Flight/Task application boundary.
func HandleCommand(ctx context.Context, tx coresync.CoreTransaction, command sharedEvent.CommandEnvelope) error {
	switch command.CommandType {
	case "probe.complete.v1":
		return HandleFoundationCommand(ctx, tx, command)
	case flighttask.CommandEmployeeReceiveTask, flighttask.CommandEmployeeStartTask, flighttask.CommandEmployeeAcceptTask, flighttask.CommandEmployeeCompleteTask:
		return flighttask.HandleEmployeeCommand(ctx, tx, command)
	case coreexception.CommandReportTaskException:
		return coreexception.HandleCommand(ctx, tx, command)
	default:
		return fmt.Errorf("unsupported core command type %q", command.CommandType)
	}
}
