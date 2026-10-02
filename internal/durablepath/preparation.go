// Preparation has an explicit, separate admission path. It never makes an
// unenrolled runtime Open succeed and cannot inherit owner-local scope from
// request contents. The existing instance-only Host seam supplies facts only.
package durablepath

import (
	"context"
	"github.com/urnetwork/connect/durablevolume"
)

func PlanPreparation(ctx context.Context, reference durablevolume.Reference, adapter durablevolume.PreparationAdapter) (durablevolume.PreparationPlan, error) {
	if ctx != nil {
		if host, present := ctx.Value(hostKey{}).(durablevolume.Host); present {
			return durablevolume.PlanPreparationWithHost(ctx, reference, adapter, host)
		}
	}
	return durablevolume.PlanPreparation(ctx, reference, adapter)
}
func PlanOwnerLocalPreparation(ctx context.Context, reference durablevolume.Reference, adapter durablevolume.PreparationAdapter) (durablevolume.PreparationPlan, error) {
	if ctx != nil {
		if host, present := ctx.Value(hostKey{}).(durablevolume.Host); present {
			return durablevolume.PlanOwnerLocalPreparationWithHost(ctx, reference, adapter, host)
		}
	}
	return durablevolume.PlanOwnerLocalPreparation(ctx, reference, adapter)
}
func ApplyPreparation(ctx context.Context, reference durablevolume.Reference, adapter durablevolume.PreparationAdapter) (durablevolume.PreparationResult, error) {
	if ctx != nil {
		if host, present := ctx.Value(hostKey{}).(durablevolume.Host); present {
			return durablevolume.ApplyPreparationWithHost(ctx, reference, adapter, host)
		}
	}
	return durablevolume.ApplyPreparation(ctx, reference, adapter)
}
func ApplyOwnerLocalPreparation(ctx context.Context, reference durablevolume.Reference, adapter durablevolume.PreparationAdapter) (durablevolume.PreparationResult, error) {
	if ctx != nil {
		if host, present := ctx.Value(hostKey{}).(durablevolume.Host); present {
			return durablevolume.ApplyOwnerLocalPreparationWithHost(ctx, reference, adapter, host)
		}
	}
	return durablevolume.ApplyOwnerLocalPreparation(ctx, reference, adapter)
}
