/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

// reconciler.go is the controller-runtime ctrl.Reconciler for
// BareMetalNode (P11-T-006 chart wire). Resolves BMC credentials from
// the referenced Secret, constructs a stub BMC client (Phase 11) keyed
// on Spec.BMC.Address scheme, and calls the pure-Go ReconcileOnce.
package controller

import (
	"context"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/bare-metal-provisioning-operator/api/v1alpha1"
	bmclient "github.com/tech88-art/O-Cloud/operators/bare-metal-provisioning-operator/internal/client"
)

// BareMetalNodeReconciler is the ctrl.Reconciler. Owns BareMetalNode +
// observes referenced Secrets for credential rotation.
type BareMetalNodeReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// SetupWithManager wires the controller into the manager.
func (r *BareMetalNodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.BareMetalNode{}).
		Complete(r)
}

// Reconcile fetches the BareMetalNode + its Secret, builds a BMC stub,
// queries BMC for observations, calls ReconcileOnce, writes Status.
func (r *BareMetalNodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("baremetalnode", req.NamespacedName)

	var bmn v1alpha1.BareMetalNode
	if err := r.Get(ctx, req.NamespacedName, &bmn); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "fetch BareMetalNode")
		return ctrl.Result{}, err
	}

	// Resolve BMC credentials from Secret.
	creds, err := r.resolveCredentials(ctx, bmn.Spec.BMC.CredentialsRef)
	if err != nil {
		logger.Error(err, "resolve BMC credentials")
		// BMCError short-circuit: BMC unreachable → return Error state
		// per P10-T-101 state machine.
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// Build BMC stub keyed on address scheme.
	bmc, err := buildBMC(bmn.Spec.BMC.Address, creds)
	if err != nil {
		logger.Error(err, "build BMC client", "address", bmn.Spec.BMC.Address)
		return ctrl.Result{RequeueAfter: 60 * time.Second}, err
	}

	// Observe BMC state.
	powerState, bmcErr := bmc.PowerStatus(ctx)
	bmcReachable := bmcErr == nil

	in := ReconcileInput{
		Current:        &bmn,
		BMCReachable:   bmcReachable,
		ImagePulled:    bmn.Status.ProvisioningState == v1alpha1.BMStateProvisioned || bmn.Status.ProvisioningState == v1alpha1.BMStateReady,
		InspectionDone: bmn.Status.ProvisioningState != v1alpha1.BMStateInspecting && bmn.Status.ProvisioningState != "",
		NodeJoined:     bmn.Status.ProvisioningState == v1alpha1.BMStateReady,
		Now:            time.Now(),
	}
	out := ReconcileOnce(in)

	if out.NextState != bmn.Status.ProvisioningState {
		bmn.Status.ProvisioningState = out.NextState
		bmn.Status.Conditions = out.Conditions
		if err := r.Status().Update(ctx, &bmn); err != nil {
			logger.Error(err, "update Status")
			return ctrl.Result{}, err
		}
		if r.Recorder != nil {
			r.Recorder.Eventf(&bmn, corev1.EventTypeNormal, "ProvisioningTransition",
				"BareMetalNode %s/%s → %s (BMC=%s power=%s)",
				bmn.Namespace, bmn.Name, out.NextState, bmn.Spec.BMC.Address, powerState)
		}
	}

	return ctrl.Result{RequeueAfter: out.RequeueAfter}, nil
}

// resolveCredentials fetches the named Secret + extracts username/password.
// Returns an error when the Secret is missing or the expected keys are absent.
func (r *BareMetalNodeReconciler) resolveCredentials(ctx context.Context, ref v1alpha1.CredentialsReference) (bmcCreds, error) {
	ns := ref.Namespace
	if ns == "" {
		ns = "ocloud-system"
	}
	var sec corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ns}, &sec); err != nil {
		return bmcCreds{}, err
	}
	u, ok1 := sec.Data["username"]
	p, ok2 := sec.Data["password"]
	if !ok1 || !ok2 {
		return bmcCreds{}, ErrMissingCredentialsKeys
	}
	return bmcCreds{Username: string(u), Password: string(p)}, nil
}

type bmcCreds struct {
	Username string
	Password string
}

// ErrMissingCredentialsKeys is returned when the BMC Secret lacks
// "username" or "password" data keys.
var ErrMissingCredentialsKeys = errMissingCredentialsKeys{}

type errMissingCredentialsKeys struct{}

func (errMissingCredentialsKeys) Error() string {
	return "BMC Secret missing required keys: username + password"
}

// buildBMC picks a stub client implementation based on the Address
// scheme prefix(redfish/redfish+https/ipmi).
func buildBMC(address string, creds bmcCreds) (bmclient.BMC, error) {
	addr := strings.ToLower(address)
	switch {
	case strings.HasPrefix(addr, "redfish"):
		return bmclient.NewRedfishStub(bmclient.RedfishConfig{
			Endpoint: address,
			Username: creds.Username,
			Password: creds.Password,
		}), nil
	case strings.HasPrefix(addr, "ipmi"):
		host, port := parseIPMIAddress(address)
		return bmclient.NewIPMIStub(bmclient.IPMIConfig{
			Host:     host,
			Port:     port,
			Username: creds.Username,
			Password: creds.Password,
		}), nil
	default:
		return nil, ErrUnsupportedBMCScheme{Address: address}
	}
}

// ErrUnsupportedBMCScheme indicates Spec.BMC.Address uses an unrecognized
// scheme prefix.
type ErrUnsupportedBMCScheme struct {
	Address string
}

func (e ErrUnsupportedBMCScheme) Error() string {
	return "unsupported BMC address scheme: " + e.Address
}

// parseIPMIAddress strips "ipmi://" prefix and splits host:port. Returns
// port 623 default when omitted.
func parseIPMIAddress(address string) (host string, port int) {
	port = 623
	addr := strings.TrimPrefix(strings.ToLower(address), "ipmi://")
	if i := strings.LastIndex(addr, ":"); i > 0 {
		host = addr[:i]
		p := addr[i+1:]
		var v int
		for _, c := range p {
			if c < '0' || c > '9' {
				return addr, 623
			}
			v = v*10 + int(c-'0')
		}
		if v > 0 {
			port = v
		}
		return host, port
	}
	return addr, port
}
