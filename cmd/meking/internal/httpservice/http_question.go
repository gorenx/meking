package httpservice

import (
	"net/http"

	querybase "github.com/memoria-space/meking/query"
	queryapplication "github.com/memoria-space/meking/query/application"
	queryquestion "github.com/memoria-space/meking/query/question"
)

func (a *httpApplication) questionSuggestions(writer http.ResponseWriter, request *http.Request) {
	var input httpSuggestionRequest
	if err := decodeHTTPJSON(writer, request, &input); err != nil {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{Code: "invalid_input", Message: err.Error()})
		return
	}
	domainRequest := queryquestion.Request{
		History: append([]string(nil), input.History...), Count: input.Count,
	}
	if err := queryquestion.ValidateRequest(domainRequest); err != nil {
		writeHTTPErrorResponse(writer, querybase.NewInvalidInputFailure(err.Error(), err), false)
		return
	}
	execution, err := a.dependencies.suggestQuestions(
		request.Context(),
		queryapplication.SuggestionRequest{History: domainRequest.History, Count: domainRequest.Count},
	)
	if err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	writeHTTPJSON(writer, http.StatusOK, newHTTPSuggestionResponse(execution))
}
