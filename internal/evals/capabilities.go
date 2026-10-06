package evals

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// ModeCapabilities is the Request.Mode of the handshake probe.
const ModeCapabilities = "capabilities"

// Declaration is what a runner says it can do: the capabilities it supports (for
// example "activation") and, for activation, the surfaces it can serve ("native").
type Declaration struct {
	Capabilities []string
	Surfaces     []string
}

// Supports reports whether the declaration lists the capability.
func (d Declaration) Supports(capability string) bool {
	return slices.Contains(d.Capabilities, capability)
}

// SurfaceReporter is implemented by runners with a fixed declaration, next to
// CapabilityReporter: the surfaces they serve in activation mode.
type SurfaceReporter interface {
	Surfaces() []string
}

// Handshaker is implemented by runners that learn what they support by asking
// (the command runner sends the probe to its command). It is consulted before
// CapabilityReporter and SurfaceReporter.
type Handshaker interface {
	Handshake(ctx context.Context) (Declaration, error)
}

// Declare returns what a runner declares: the answer of its handshake, else its
// static capabilities and surfaces. A runner with none declares nothing.
func Declare(ctx context.Context, r Runner) (Declaration, error) {
	if h, ok := r.(Handshaker); ok {
		return h.Handshake(ctx)
	}
	var d Declaration
	if reporter, ok := r.(CapabilityReporter); ok {
		d.Capabilities = reporter.Capabilities()
	}
	if reporter, ok := r.(SurfaceReporter); ok {
		d.Surfaces = reporter.Surfaces()
	}
	return d, nil
}

// RequireSurface refuses a runner that does not declare capability on surface, so
// a mode is never run as something else: an old runner that ignored an activation
// request would otherwise run full cases and report their results as activation
// figures. An empty surface only checks the capability.
func RequireSurface(ctx context.Context, r Runner, capability, surface string) error {
	if r == nil {
		return fmt.Errorf("no runner configured for %s mode", capability)
	}
	d, err := Declare(ctx, r)
	if err != nil {
		return fmt.Errorf("runner %q: capabilities handshake failed: %w", r.Name(), err)
	}
	if !d.Supports(capability) {
		return fmt.Errorf("runner %q does not support %s mode: it does not declare the %q capability", r.Name(), capability, capability)
	}
	if surface != "" && !slices.Contains(d.Surfaces, surface) {
		return fmt.Errorf("runner %q does not support the %s surface of %s mode: it declares %s", r.Name(), surface, capability, surfaceList(d.Surfaces))
	}
	return nil
}

func surfaceList(surfaces []string) string {
	if len(surfaces) == 0 {
		return "no surface"
	}
	return "the " + strings.Join(surfaces, ", ") + " surface"
}
