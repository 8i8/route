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
	"strings"
)

// Middleware are functions that receive and return an http.HandlerFunc
type Middleware func(http.HandlerFunc) http.HandlerFunc

// route comprises of a path and HandlerFunc.
type route struct {
	path        string
	handlerFunc http.HandlerFunc
}

// Handle returns a route
func Handle(path string, handle http.Handler) route {
	return route{path, wrap(handle)}
}

// Wrap wraps a route with the provided http.HandlerFunc
func (r route) Wrap(fn func(http.HandlerFunc) http.HandlerFunc) route {
	return route{r.path, fn(r.handlerFunc)}
}

// Group simplifies route composition by permitting the selective and
// collective application of middleware. Middleware once applied wraps all
// endpoints that are applied after it. Groups can be added to groups as
// subgroups, enabling the selective application of middleware to subgroups
// within a group rather than globally.
type Group struct {
	Mux    *http.ServeMux
	mwares []Middleware
	routes []route
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

func what(t any) string {
	return fmt.Sprintf(": (%T, %v)", t, t)
}

func wrap(h http.Handler) http.HandlerFunc {
	if h == nil {
		_ = log.Output(2, fmt.Sprintf("%s:nil http.Handler", fname()))
		os.Exit(1)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
	}
}

func fname() string {
	pc, _, _, _ := runtime.Caller(1)
	fullName := runtime.FuncForPC(pc).Name()
	parts := strings.Split(fullName, ".")
	return parts[len(parts)-1] // Extract only the function name
}

// exit enables testing of code that calls os.Exit
var exit = func(code int) {
	os.Exit(code)
}

func (g *Group) compose() *Group {
	for i := range g.routes {

		// Reverse index to achieve first in first applied behavior.
		reverseIndex := len(g.routes) - 1 - i
		hFunc := g.routes[reverseIndex].handlerFunc
		path := g.routes[reverseIndex].path
		if hFunc == nil {
			// nil values in nested middleware can be very tricky to deal so we
			// get out fast and check everywhere.
			_ = log.Output(2, fmt.Sprintf("%s:%s", fname(), errNilFunc))
			exit(1)
		}

		// Apply each middleware to our function
		for i := range g.mwares {
			// Reverse again
			reverseIndexJ := len(g.mwares) - 1 - i
			hFunc = g.mwares[reverseIndexJ](hFunc)

			// Check for nil middleware output
			if hFunc == nil {
				_ = log.Output(2, fmt.Sprintf("%s:%s", fname(), errFuncReturnNil))
				exit(1)
			}
		}

		if g.Mux == nil {
			// No server just yet, we need to replace the function with its
			// wrapped replacement.
			g.routes[reverseIndex].handlerFunc = hFunc
		} else {
			// This is the final compilation, add the HandlerFunc to the server.
			g.Mux.HandleFunc(path, hFunc)
		}
	}
	return g
}

// Compile wraps all routes with the appropriate middleware and loads them all
// into a multiplex server.
func (g *Group) Compile() *http.ServeMux {
	if g.Mux == nil {
		g.Mux = &http.ServeMux{}
	}
	g = g.compose()
	return g.Mux
}

// Wrap wraps all endpoints in a Group with its provided decorators, they are
// applied in order, first in first out.
func (g *Group) Wrap(mw ...any) *Group {

	for _, obj := range mw {
		switch t := obj.(type) {
		case Middleware:
			if t == nil {
				_ = log.Output(2, fmt.Sprintf("%s:%s", fname(), errNilMiddleware))
				exit(1)
			}
			g.mwares = append(g.mwares, t)
		case func(http.HandlerFunc) http.HandlerFunc:
			g.mwares = append(g.mwares, t)
		case http.Handler:
			if t == nil {
				_ = log.Output(2, fmt.Sprintf("%s:%s", fname(), errNilHandler))
				exit(1)
			}
			_ = log.Output(2, fmt.Sprintf("%s:%s", fname(), errHandlerUsed))
			exit(1)
		default:
			_ = log.Output(2, fmt.Sprintf("%s:%s:%s", fname(), errSwitchDefault, what(t)))
			exit(1)
		}
	}

	return g
}

// Handle expects either *route.Group, or string http.Handler, string
// http.HandlerFunc pairs. Middleware applied to subgroups remains exclusive to
// the subgroup.
func (g *Group) Handle(h ...any) *Group {
	var path string
	var offset int
	for i, obj := range h {
		switch t := obj.(type) {
		case Group:
			_ = log.Output(2, fmt.Sprintf("%s:%s", fname(), errGroupUsed))
			exit(1)
		case *Group:
			if t == nil {
				_ = log.Output(2, fmt.Sprintf("%s:%s", fname(), errNilGroup))
				exit(1)
			}
			// Counting a group, we need to offset the modulo 2 calculation.
			offset++
			t = t.compose()
			g.routes = append(g.routes, t.routes...)
		case http.HandlerFunc:
			if t == nil {
				_ = log.Output(2, fmt.Sprintf("%s:%s", fname(), errNilHandlerFunc))
				exit(1)
			}
			if (i+offset)%2 != 1 {
				_ = log.Output(2, fmt.Sprintf("%s:%s", fname(), errHandleFormat))
				exit(1)
			}
			g.routes = append(g.routes, route{path, t})
		case http.Handler:
			if t == nil {
				_ = log.Output(2, fmt.Sprintf("%s:%s", fname(), errNilHandler))
				exit(1)
			}
			if (i+offset)%2 != 1 {
				_ = log.Output(2, fmt.Sprintf("%s:%s", fname(), errHandleFormat))
				exit(1)
			}
			g.routes = append(g.routes, route{path, wrap(t)})
		case func(w http.ResponseWriter, r *http.Request):
			if (i+offset)%2 != 1 {
				_ = log.Output(2, fmt.Sprintf("%s:%s", fname(), errHandleFormat))
				exit(1)
			}
			g.routes = append(g.routes, route{path, t})
		// Test for string last, a typed string might implement ServeHTTP
		case string:
			// Strings must be first of a pair
			if (i+offset)%2 != 0 {
				_ = log.Output(2, fmt.Sprintf("%s:%s", fname(), errHandleFormat))
				exit(1)
			}
			path = t
		default:
			_ = log.Output(2, fmt.Sprintf("%s:%s:%s", fname(), errSwitchDefault, what(t)))
			exit(1)
		}
	}
	return g
}
