// Package application owns Core use-case composition. Concrete business
// modules are added only when their Foundation slice is approved.
package application

type Boundary struct {
	Name string
}

func NewBoundary() Boundary { return Boundary{Name: "core-modular-monolith"} }
