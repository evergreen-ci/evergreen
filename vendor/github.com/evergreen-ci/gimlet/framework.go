package gimlet

import (
	"context"
	"net/http"
)

// RouteHandler provides an alternate method for defining routes with
// the goals of separating the core operations of handling a rest result.
type RouteHandler interface {
	// Factory produces, this makes it possible for you to store
	// request-scoped data in the implementation of the Handler
	// rather than attaching data to the context. The factory
	// allows gimlet to, internally, reconstruct a handler interface
	// for every request.
	//
	// Factory is always called at the beginning of the request.
	Factory() RouteHandler

	// Parse makes it possible to modify the request context and
	// populate the implementation of the RouteHandler. This also
	// allows you to isolate your interaction with the request
	// object.
	Parse(context.Context, *http.Request) (context.Context, error)

	// Runs the core buinsess logic for the route, returning a
	// Responder interface to provide structure around returning
	//
	// Run methods do not return an error. Implementors are
	// responsible for forming a response, even in error cases.
	Run(context.Context) Responder
}

// handleHandler converts a RouteHandler implementation into a
// standard go http.HandlerFunc.
func handleHandler(h RouteHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var err error

		handler := h.Factory()
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		ctx, err = handler.Parse(ctx, r)
		if err != nil {
			e := getError(err, http.StatusBadRequest)
			WriteJSONResponse(w, e.StatusCode, e)
			return
		}

		resp := handler.Run(ctx)
		if resp == nil {
			e := ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    "undefined response",
			}
			WriteJSONResponse(w, e.StatusCode, e)
			return
		}

		if err := resp.Validate(); err != nil {
			e := getError(err, http.StatusBadRequest)
			WriteJSONResponse(w, e.StatusCode, e)
			return
		}

		// if this response is paginated, add the appropriate metadata.
		if resp.Pages() != nil {
			w.Header().Set("Link", resp.Pages().GetLinks(r.URL.Path))
		}

		// Write the response, based on the format specified.
		switch resp.Format() {
		case JSON:
			WriteJSONResponse(w, resp.Status(), resp.Data())
		case TEXT:
			WriteTextResponse(w, resp.Status(), resp.Data())
		case HTML:
			WriteHTMLResponse(w, resp.Status(), resp.Data())
		case YAML:
			WriteYAMLResponse(w, resp.Status(), resp.Data())
		case BINARY:
			WriteBinaryResponse(w, resp.Status(), resp.Data())
		}
	}
}
