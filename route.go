// route provides a simple tool set for grouping endpoints wrapping multiple
// routes with middleware. Middleware that is applied prior to the addition of
// a route, is applied to that route. Groups can be passed into groups and any
// middleware that has been applied to a subgroup is not applied to the whole,
// greatly facilitating the organisation of middleware application.
package route

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
)

var (
	errSwitchDefault = errors.New("switch default, unknown type")
	errNilFunc       = errors.New("nil returned from HandlerFunc in chain")
	errFuncReturnNil = errors.New("function returned nil")
)

// exit func permits that the exit function be overriden during testing.
var exit func(error)

// exitWithError logs the error message and exits with code 1, opting for hard
// exit on error as routes code is only run at startup.
func exitWithError(err error) {
	_, file, line, _ := runtime.Caller(1) // Get caller info
	_ = log.Output(3, file+":"+strconv.Itoa(line)+": "+err.Error())
	os.Exit(1)
}

func init() {
	exit = exitWithError
}

// Routable provides a collection of routes.
type Routable interface {
	Routes() []Handler
}

// Group returned by NewGroup works through function composition to group
// together routes and to wrap all routes within the group with any provided
// middleware.
type Group interface {
	Compose(...Routable) *http.ServeMux
	Add(...Routable) *group
	Wrap(...Middleware) *group
	Routable
}

// Middleware is a shorthand for func(http.Handler) http.Handler the signature
// of route middleware.
type Middleware func(http.Handler) http.Handler

// Handler comprises of a path and handler defining a route.
type Handler struct {
	Path    string
	Handler http.Handler
}

// Routes enables Route to meet the Router interface.
func (h Handler) Routes() []Handler {
	return []Handler{h}
}

// Handle returns a route.
func Handle(path string, handle http.Handler) Handler {
	return Handler{path, handle}
}

// Wrap wraps a route with the provided Middleware.
func (h Handler) Wrap(funcs ...Middleware) Handler {
	for i := len(funcs) - 1; i >= 0; i-- {
		h.Handler = funcs[i](h.Handler)
	}
	return Handler{h.Path, h.Handler}
}

// group simplifies route composition by permitting the selective and
// collective application of middleware. Middleware once applied wraps all
// endpoints that are applied after it. Groups can be added to groups as
// subgroups, enabling the selective application of middleware to subgroups
// within a group rather than globally.
type group struct {
	mux    *http.ServeMux
	mwares []Middleware
	routes []Handler
}

// NewGroup retuns a route group.
func NewGroup(routes ...Routable) *group {
	g := &group{}
	g = g.Add(routes...)
	return g
}

// Add adds Routables, either route.Group's or route.Handler's to the group.
func (g *group) Add(h ...Routable) *group {
	for _, obj := range h {
		switch t := obj.(type) {
		case Handler:
			g.routes = append(g.routes, t)
		case *group:
			g.routes = append(g.routes, t.Routes()...)
		default:
			exit(fmt.Errorf("%T:%w", t, errSwitchDefault))
		}
	}
	return g
}

// Wrap wraps all endpoints in a Group with its provided decorators, they are
// applied in order, first in first out.
func (g *group) Wrap(mw ...Middleware) *group {
	g.mwares = append(g.mwares, mw...)
	return g
}

// Routes returns the routes within a route group with all middleware applied.
func (g *group) Routes() []Handler {
	for i := range g.routes {

		// Reverse index to achieve first in first applied behaviour.
		reverseIndex := len(g.routes) - 1 - i
		handler := g.routes[reverseIndex].Handler
		if handler == nil {
			// nil values in nested middleware can be very tricky to deal so we
			// get out fast and check everywhere.
			exit(errNilFunc)
		}

		// Apply each middleware to our function
		for i := range g.mwares {
			// Reverse again
			reverseIndexJ := len(g.mwares) - 1 - i
			handler = g.mwares[reverseIndexJ](handler)

			// Check for nil middleware output
			if handler == nil {
				exit(errFuncReturnNil)
			}
		}

		// No server just yet, we need to replace the function with its
		// wrapped replacement.
		g.routes[reverseIndex].Handler = handler
	}
	return g.routes
}

// Mux sets the given *http.ServeMux as its server, use when compile is called.
func (g *group) Mux(m *http.ServeMux) {
	g.mux = m
}

// Compose wraps all routes with the middleware and loads them into either the
// provided multiplex server or a default http.ServeMux.
func (g *group) Compose(routes ...Routable) *http.ServeMux {
	if g.mux == nil {
		g.mux = &http.ServeMux{}
	}
	g = g.Add(routes...)
	for _, route := range g.Routes() {
		g.mux.Handle(route.Path, route.Handler)
	}
	return g.mux
}
