// Package application owns Edge use-case composition around projections and
// commands; it never owns Core business truth.
package application

type Boundary struct {
	Name string
}

func NewBoundary() Boundary { return Boundary{Name: "edge-projection-gateway"} }
