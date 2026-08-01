package httpapi

import (
	"net/http"
	"sort"
	"strings"
)

type HandlerFunc func(http.ResponseWriter, *http.Request) error

type Router struct {
	mux     *http.ServeMux
	methods map[string]map[string]struct{}
}

func NewRouter() *Router {
	router := &Router{mux: http.NewServeMux(), methods: make(map[string]map[string]struct{})}
	router.mux.HandleFunc("/", func(w http.ResponseWriter, request *http.Request) {
		WriteError(w, request, &Error{
			Status:  http.StatusNotFound,
			Code:    CodeNotFound,
			Message: "resource not found",
		})
	})
	return router
}

func (r *Router) Handle(method, path string, handler HandlerFunc) {
	r.HandleHTTP(method, path, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if err := handler(w, request); err != nil {
			WriteError(w, request, err)
		}
	}))
}

func (r *Router) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	method, path, found := strings.Cut(pattern, " ")
	if !found || method == "" || path == "" {
		panic("httpapi: route pattern must include method and path")
	}
	r.HandleHTTP(method, path, http.HandlerFunc(handler))
}

func (r *Router) HandleHTTP(method, path string, handler http.Handler) {
	methods, exists := r.methods[path]
	if !exists {
		methods = make(map[string]struct{})
		r.methods[path] = methods
		r.mux.HandleFunc(path, func(w http.ResponseWriter, request *http.Request) {
			allowed := make([]string, 0, len(methods))
			for allowedMethod := range methods {
				allowed = append(allowed, allowedMethod)
			}
			sort.Strings(allowed)
			w.Header().Set("Allow", strings.Join(allowed, ", "))
			WriteError(w, request, &Error{
				Status:  http.StatusMethodNotAllowed,
				Code:    CodeMethodNotAllowed,
				Message: "method not allowed",
			})
		})
	}
	methods[method] = struct{}{}
	r.mux.Handle(method+" "+path, handler)
}

func (r *Router) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	r.mux.ServeHTTP(w, request)
}

func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		defer func() {
			if recover() != nil {
				WriteError(w, request, internalError(nil))
			}
		}()
		next.ServeHTTP(w, request)
	})
}
