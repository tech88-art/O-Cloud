/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package publisher

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	resourceapi "k8s.io/api/resource/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
	"github.com/tech88-art/O-Cloud/operators/npu-dra-driver/internal/source"
)

// DefaultRequeueInterval is the publisher's baseline reconcile cadence. The
// publisher requeues every 30s even without a Source event so stale slices
// from a previous run get cleaned up promptly on a fresh process start.
const DefaultRequeueInterval = 30 * time.Second

// SliceLabelManagedBy is the label set on every ResourceSlice the publisher
// owns. The cleanup loop uses this to scope its delete-stale pass — it never
// touches slices owned by other drivers.
const SliceLabelManagedBy = "npu.ocloud.edge.example.com/managed-by"

// SliceLabelManagedByValue is the value of the SliceLabelManagedBy label.
const SliceLabelManagedByValue = "npu-dra-driver"

// Publisher runs the ResourceSlice publication loop. Its lifecycle:
//
//  1. Reconcile: List from Source → diff against live ResourceSlices owned
//     by this driver → upsert changed → delete stale → schedule next tick.
//  2. Source.Watch: when an Event arrives, Reconcile fires early (short-
//     circuits the timer).
//  3. ctx cancellation: graceful exit. Owner-less slices are NOT deleted on
//     shutdown — they are recreated on next startup so kind smoke tests can
//     restart the manager without the driver name flapping out.
type Publisher struct {
	// Client is a controller-runtime client wired to a manager. Must be
	// authoritative for resource.k8s.io/v1beta1 ResourceSlice get/list/
	// create/update/delete. RBAC for this lands at P4-T-101 helm chart.
	Client client.Client

	// Source produces the device inventory. Required.
	//
	// Phase 7 P7-T-004 (ADR-0011 §2) lifted this interface OUT of this
	// package into internal/source/. Phase 4-6 SimulatorSource is now
	// mockjson.MockJSONSource; cmd/main.go constructs the right impl via
	// the --source-type flag + selectSource dispatch.
	Source source.Source

	// RequeueInterval overrides the publisher's reconcile cadence. Zero
	// means use DefaultRequeueInterval (30s). Tests can shrink this.
	RequeueInterval time.Duration

	// Log is the publisher's logger. Defaults to controller-runtime's
	// global logger when nil.
	Log logr.Logger
}

// Start runs the publisher loop until ctx is cancelled. It blocks; intended
// to be wired into mgr.Add via a wrapper that satisfies controller-runtime's
// manager.Runnable. Phase 4 T005 ships the loop; T006 may add a separate
// Runnable for the claim controller.
func (p *Publisher) Start(ctx context.Context) error {
	if p.Client == nil {
		return errors.New("publisher: Client is nil")
	}
	if p.Source == nil {
		return errors.New("publisher: Source is nil")
	}
	lg := p.Log
	if lg.GetSink() == nil {
		lg = log.FromContext(ctx).WithName("npu-dra-publisher")
	}
	requeue := p.RequeueInterval
	if requeue <= 0 {
		requeue = DefaultRequeueInterval
	}

	lg.Info("Publisher starting", "requeue", requeue)
	watch := p.Source.Watch(ctx)

	// Run an initial reconcile immediately on startup.
	if err := p.Reconcile(ctx); err != nil {
		lg.Error(err, "Initial reconcile failed (will retry on next tick)")
	}

	t := time.NewTicker(requeue)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			lg.Info("Publisher stopping", "reason", "context cancelled")
			return nil
		case <-t.C:
			if err := p.Reconcile(ctx); err != nil {
				lg.Error(err, "Reconcile failed (will retry on next tick)")
			}
		case ev, ok := <-watch:
			if !ok {
				lg.V(1).Info("Source watch channel closed; relying on tick alone")
				watch = nil
				continue
			}
			lg.V(1).Info("Source event; reconciling", "source", ev.Source, "reason", ev.Reason)
			if err := p.Reconcile(ctx); err != nil {
				lg.Error(err, "Reconcile failed (will retry on next tick)")
			}
		}
	}
}

// Reconcile performs one list-diff-upsert-delete-stale pass. Exposed for
// tests; production callers go through Start.
func (p *Publisher) Reconcile(ctx context.Context) error {
	lg := p.Log
	if lg.GetSink() == nil {
		lg = log.FromContext(ctx).WithName("npu-dra-publisher")
	}

	desired, err := p.Source.List(ctx)
	if err != nil {
		return fmt.Errorf("publisher reconcile: source list: %w", err)
	}

	// List live ResourceSlices owned by this driver. We label-filter via
	// SliceLabelManagedBy so a future inference-operator publisher (or a
	// third-party driver) can co-exist without churn.
	var live resourceapi.ResourceSliceList
	if err := p.Client.List(ctx, &live, client.MatchingLabels{SliceLabelManagedBy: SliceLabelManagedByValue}); err != nil {
		return fmt.Errorf("publisher reconcile: list live slices: %w", err)
	}

	liveByName := make(map[string]resourceapi.ResourceSlice, len(live.Items))
	for _, sl := range live.Items {
		liveByName[sl.Name] = sl
	}

	desiredByName := make(map[string]resourceapi.ResourceSlice, len(desired))
	for _, nd := range desired {
		sl := p.buildSlice(nd)
		desiredByName[sl.Name] = sl
	}

	// Upsert: create or update each desired slice.
	for name, want := range desiredByName {
		got, exists := liveByName[name]
		if !exists {
			if err := p.Client.Create(ctx, &want); err != nil {
				if apierrors.IsAlreadyExists(err) {
					lg.V(1).Info("Race: slice exists; will reconcile next tick", "slice", name)
					continue
				}
				return fmt.Errorf("publisher reconcile: create slice %q: %w", name, err)
			}
			lg.V(1).Info("Created ResourceSlice", "slice", name, "devices", len(want.Spec.Devices))
			continue
		}
		if sliceSpecEqual(got.Spec, want.Spec) {
			continue
		}
		got.Spec = want.Spec
		if got.Labels == nil {
			got.Labels = map[string]string{}
		}
		got.Labels[SliceLabelManagedBy] = SliceLabelManagedByValue
		if err := p.Client.Update(ctx, &got); err != nil {
			return fmt.Errorf("publisher reconcile: update slice %q: %w", name, err)
		}
		lg.V(1).Info("Updated ResourceSlice", "slice", name, "devices", len(want.Spec.Devices))
	}

	// Delete stale: any live slice whose name is not in the desired set.
	for name, got := range liveByName {
		if _, keep := desiredByName[name]; keep {
			continue
		}
		if err := p.Client.Delete(ctx, &got); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("publisher reconcile: delete stale slice %q: %w", name, err)
		}
		lg.V(1).Info("Deleted stale ResourceSlice", "slice", name)
	}

	if len(desiredByName) == 0 {
		lg.Info("Reconcile complete (no devices discovered)", "live", len(liveByName))
	}
	return nil
}

// buildSlice constructs the desired ResourceSlice for one node's devices.
// Owner refs left blank per P4-T-005 acceptance — Phase 5 may add ownership
// when inference-operator consumes claims.
func (p *Publisher) buildSlice(nd source.NodeDevices) resourceapi.ResourceSlice {
	devs := make([]resourceapi.Device, 0, len(nd.Devices))
	for _, d := range nd.Devices {
		devs = append(devs, d.ToUpstream())
	}
	return resourceapi.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: sliceNameForNode(nd.NodeName),
			Labels: map[string]string{
				SliceLabelManagedBy: SliceLabelManagedByValue,
			},
		},
		Spec: resourceapi.ResourceSliceSpec{
			Driver: v1alpha1.DriverName,
			Pool: resourceapi.ResourcePool{
				Name:               nd.NodeName,
				Generation:         1,
				ResourceSliceCount: 1,
			},
			NodeName: nd.NodeName,
			Devices:  devs,
		},
	}
}

// sliceNameForNode is the deterministic ResourceSlice name for one node.
// Stable across restarts — same node always gets the same slice name so the
// Reconcile diff fires update (not create+delete) when device contents change.
func sliceNameForNode(node string) string {
	return "npu-dra-" + node
}

// sliceSpecEqual compares the publisher-owned subset of two ResourceSliceSpec
// values: Driver + Pool + NodeName + Devices. We deliberately ignore
// PerDeviceNodeSelection, AllNodes, etc. because Phase 4 doesn't set them —
// any drift on those fields is owned upstream (kubelet / scheduler) and not
// our concern.
func sliceSpecEqual(a, b resourceapi.ResourceSliceSpec) bool {
	if a.Driver != b.Driver {
		return false
	}
	if !reflect.DeepEqual(a.Pool, b.Pool) {
		return false
	}
	if a.NodeName != b.NodeName {
		return false
	}
	if !reflect.DeepEqual(a.Devices, b.Devices) {
		return false
	}
	return true
}

// SetupWithManager wires the Publisher as a manager Runnable. Call from main
// (cmd/main.go) when --enable-publisher is set.
func (p *Publisher) SetupWithManager(mgr ctrl.Manager) error {
	return mgr.Add(runnableFunc(p.Start))
}

type runnableFunc func(ctx context.Context) error

func (r runnableFunc) Start(ctx context.Context) error { return r(ctx) }
