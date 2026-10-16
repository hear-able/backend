package jsonresponder

import apperrors "hearable/backend/package/errors"

type (
	SimpleMessage struct {
		Message string `json:"message"`
	}

	ErrorBody struct {
		Error *apperrors.AppError `json:"error"`
	}
)
