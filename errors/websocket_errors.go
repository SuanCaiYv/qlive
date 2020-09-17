package errors

const (
	WSErrorOK             = 0
	WSErrorUnknownMessage = 10001
	WSErrorTokenInvalid   = 10002
	WSErrorNoPermission   = 10003
	WSErrorRoomNoExist    = 10011
	WSErrorRoomInPK       = 10012
	WSErrorRoomNotInPK    = 10013
)

var WSErrorToString = map[int]string{
	WSErrorOK:             "",
	WSErrorUnknownMessage: "unknown message",
	WSErrorTokenInvalid:   "token invalid",
	WSErrorNoPermission:   "no permission",
	WSErrorRoomNoExist:    "room no exist",
	WSErrorRoomInPK:       "room in PK",
	WSErrorRoomNotInPK:    "room not in PK",
}

type WSError struct {
	s string
}

func (e WSError) Error() string {
	return e.s
}

func NewWSError(errString string) error {
	return &WSError{errString}
}
