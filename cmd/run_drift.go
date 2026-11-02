package cmd

import "github.com/chazu/pudl/internal/acute"

// These aliases preserve the command package's report surface while keeping
// drift values independent of Cobra and subprocess adapters.
type ResourceDrift = acute.ResourceDrift
type ModelDriftResult = acute.ModelDriftResult
