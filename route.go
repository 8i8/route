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

// Router provides a collection of routes.
type Router interface {
	Routes() []Route
}

// Middleware is a shorthand for func(http.Handler) http.Handler the signature
// of route middleware.
type Middleware func(http.Handler) http.Handler

// Route comprises of a path and handler.
type Route struct {
	Path    string
	Handler http.Handler
}

// Routes enables Route to meet the Router interface.
func (r Route) Routes() []Route {
	return []Route{r}
}

// Define returns a route.
func Define(path string, handle http.Handler) Route {
	return Route{path, handle}
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
func (r Route) Wrap(funcs ...Middleware) Route {
	for i := len(funcs) - 1; i >= 0; i-- {
		r.Handler = funcs[i](r.Handler)
	}
	return Route{r.Path, r.Handler}
}

// Group simplifies route composition by permitting the selective and
// collective application of middleware. Middleware once applied wraps all
// endpoints that are applied after it. Groups can be added to groups as
// subgroups, enabling the selective application of middleware to subgroups
// within a group rather than globally.
type Group struct {
	Mux    *http.ServeMux
	mwares []Middleware
	routes []Route
}

func NewGroup() *Group {
	return &Group{}
}

var (
	errHandlerUsed    = errors.New("http.Handle passed into middleware Wrap")
	errHandleFormat   = errors.New("format err, should be (<path>, <handler>) pairs")
	errSwitchDefault  = errors.New("switch default, unknown type")
	errNilFunc        = errors.New("nil returned from HandlerFunc in chain")
	errNilHandlerFunc = errors.New("nil HandlerFunc")
	errNilHandler     = errors.New("nil http.Handler")
	errNilMiddleware  = errors.New("nil route.Middleware")
	errNilGroup       = errors.New("nil route.Group")
	errFuncReturnNil  = errors.New("function returned nil")
	errGroupUsed      = errors.New("want *route.Group not route.Group")
)

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

func (g *Group) Routes() []Route {
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
func (g *Group) Compile() *http.ServeMux {
	if g.Mux == nil {
		g.Mux = &http.ServeMux{}
	}
	for _, route := range g.Routes() {
		g.Mux.Handle(route.Path, route.Handler)
	}
	return g.Mux
}

// Wrap wraps all endpoints in a Group with its provided decorators, they are
// applied in order, first in first out.
func (g *Group) Wrap(mw ...Middleware) *Group {
	if mw == nil || len(mw) > 0 && mw[0] == nil {
		exit(errNilMiddleware)
	}
	g.mwares = append(g.mwares, mw...)
	return g
}

// Handle expects either *route.Group, or string http.Handler, string
// http.HandlerFunc pairs. Middleware applied to subgroups remains exclusive to
// the subgroup.
func (g *Group) Handle(h ...Router) *Group {
	for _, obj := range h {
		switch t := obj.(type) {
		case Route:
			g.routes = append(g.routes, t)
		case *Group:
			g.routes = append(g.routes, t.Routes()...)
		default:
			exit(fmt.Errorf("%T:%w", t, errSwitchDefault))
		}
	}
	return g
}
