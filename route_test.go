package route

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBasicRouteHandling(t *testing.T) {
	g := NewGroup()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Hello, World!"))
	})
	g.Add(Handle("/hello", handler))

	mux := g.Compose()

	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	expectedBody := "Hello, World!"
	body := w.Body.String()
	if body != expectedBody {
		t.Fatalf("Expected body %q, got %q", expectedBody, body)
	}
}

func TestMiddlewareApplication(t *testing.T) {
	g := NewGroup()

	// Middleware that adds a header
	addHeader := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Custom-Header", "MiddlewareApplied")
			next.ServeHTTP(w, r)
		})
	}

	// Middleware that modifies response body
	modifyBody := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("Modified: "))
			next.ServeHTTP(w, r)
		})
	}

	g.Wrap(addHeader, modifyBody)

	g.Add(Handle("/test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("OriginalResponse"))
	})))

	mux := g.Compose()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	// Check if middleware added the header
	if resp.Header.Get("X-Custom-Header") != "MiddlewareApplied" {
		t.Fatalf("Expected header 'X-Custom-Header' to be 'MiddlewareApplied', but got %q", resp.Header.Get("X-Custom-Header"))
	}

	// Check if middleware modified the response body correctly
	expectedBody := "Modified: OriginalResponse"
	body := w.Body.String()
	if body != expectedBody {
		t.Fatalf("Expected response body %q, got %q", expectedBody, body)
	}
}

func TestSubgroupIsolation(t *testing.T) {
	g := NewGroup()
	sub := NewGroup()

	sub.Wrap(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Subgroup", "Applied")
			next.ServeHTTP(w, r)
		})
	})

	sub.Add(Handle("/sub", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte("Subgroup"))
		if err != nil {
			t.Errorf("error:%s", err)
		}
	})))

	g.Add(sub) // Add the subgroup to the main group

	mux := g.Compose()

	req := httptest.NewRequest(http.MethodGet, "/sub", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.Header.Get("X-Subgroup") != "Applied" {
		t.Fatalf("Expected header 'X-Subgroup' to be 'Applied', but got %q", resp.Header.Get("X-Subgroup"))
	}
}
