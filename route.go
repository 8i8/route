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
	"strings"
)

var (
	errSwitchDefault = errors.New("switch default, unknown type")
	errNilFunc       = errors.New("nil returned from HandlerFunc in chain")
	errFuncReturnNil = errors.New("function returned nil")
)

// Router provides a collection of routes.
type Router interface {
	Routes() []Handler
}

// Group returned by NewGroup works through function composition to group
// together routes and to wrap all routes within the group with any provided
// middleware.
type Group interface {
	Compile() *http.ServeMux
	Add(...Handler)
	Wrap(...Middleware)
	Router
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
func (r Handler) Routes() []Handler {
	return []Handler{r}
}

// Handle returns a route.
func Handle(path string, handle http.Handler) Handler {
	return Handler{path, handle}
}

// Wrap wraps middleware, returning a single function with all the provided
// functions chained within it, the function are reversed in order so that the first in is the first applied.
func Wrap(mw ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		for i := len(mw) - 1; i >= 0; i-- {
			next = mw[i](next)
		}
		return next
	}
}

// Wrap wraps a route with the provided Middleware.
func (r Handler) Wrap(funcs ...Middleware) Handler {
	for i := len(funcs) - 1; i >= 0; i-- {
		r.Handler = funcs[i](r.Handler)
	}
	return Handler{r.Path, r.Handler}
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

// Mux sets the given *http.ServeMux as its server, use when compile is called.
func (g *group) Mux(m *http.ServeMux) {
	g.mux = m
}

// NewGroup retuns a route group.
func NewGroup(routes ...Handler) *group {
	return &group{routes: routes}
}

// exitWithLog logs the error message and exits with code 0.
func exitWithLog(msg string) {
	_, file, line, _ := runtime.Caller(1) // Get caller info
	_ = log.Output(3, file+":"+strconv.Itoa(line)+": "+msg)
	os.Exit(0)
}

// exitWithError logs the error message and exits with code 1.
func exitWithError(err error) {
	_, file, line, _ := runtime.Caller(1) // Get caller info
	_ = log.Output(3, file+":"+strconv.Itoa(line)+": "+err.Error())
	os.Exit(1)
}

var exit func(error)

func init() {
	exit = exitWithError
}

func fname() string {
	pc, _, _, _ := runtime.Caller(1)
	fullName := runtime.FuncForPC(pc).Name()
	parts := strings.Split(fullName, ".")
	return parts[len(parts)-1] // Extract only the function name
}

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

// Compile wraps all routes with the appropriate middleware and loads them all
// into a multiplex server.
func (g *group) Compile(routes ...Handler) *http.ServeMux {
	if g.mux == nil {
		g.mux = &http.ServeMux{}
	}
	g.routes = append(g.routes, routes...)
	for _, route := range g.Routes() {
		g.mux.Handle(route.Path, route.Handler)
	}
	return g.mux
}

// Wrap wraps all endpoints in a Group with its provided decorators, they are
// applied in order, first in first out.
func (g *group) Wrap(mw ...Middleware) *group {
	g.mwares = append(g.mwares, mw...)
	return g
}

// Add expects either a route.Group, or route, returned by route.Handle.
func (g *group) Add(h ...Router) *group {
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
