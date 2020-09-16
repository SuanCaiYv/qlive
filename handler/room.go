package handler

import (
	"math/rand"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/qiniu/x/xlog"

	"github.com/qrtc/qlive/errors"
	"github.com/qrtc/qlive/protocol"
)

// RoomHandler 处理直播间的CRUD，以及进入、退出房间等操作。
type RoomHandler struct {
	Room     RoomInterface
	LiveHost string
	LiveHub  string
}

// RoomInterface 处理房间相关API的接口。
type RoomInterface interface {
	CreateRoom(xl *xlog.Logger, room *protocol.LiveRoom) error
	// ListAllRooms 列出全部正在直播的房间列表。
	ListAllRooms(xl *xlog.Logger) ([]protocol.LiveRoom, error)
	// ListPKRooms 列出可以与userID PK的房间列表。
	ListPKRooms(xl *xlog.Logger, userID string) ([]protocol.LiveRoom, error)
	// CloseRoom 关闭直播间。
	CloseRoom(xl *xlog.Logger, userID string, roomID string) error
}

// ListRooms 列出房间请求。
func (h *RoomHandler) ListRooms(c *gin.Context) {

	if c.Query("can_pk") == "true" {
		h.ListCanPKRooms(c)
		return
	}

	h.ListAllRooms(c)
}

// ListCanPKRooms 列出当前主播可以PK的房间列表。
func (h *RoomHandler) ListCanPKRooms(c *gin.Context) {
	xl := c.MustGet(protocol.XLogKey).(*xlog.Logger)
	userID := c.GetString(protocol.UserIDContextKey)
	rooms, err := h.Room.ListPKRooms(xl, userID)
	if err != nil {
		xl.Errorf("failed to list rooms which can be PKed, error %v", err)

	}
	resp := &protocol.ListRoomsResponse{}
	for _, room := range rooms {
		resp.Rooms = append(resp.Rooms, protocol.GetRoomResponse{
			ID:   room.ID,
			Name: room.Name,
		})
	}
}

// ListAllRooms 列出全部房间。
func (h *RoomHandler) ListAllRooms(c *gin.Context) {

}

// CreateRoom 创建直播间。
func (h *RoomHandler) CreateRoom(c *gin.Context) {
	xl := c.MustGet(protocol.XLogKey).(*xlog.Logger)
	requestID := xl.ReqId
	userID := c.GetString(protocol.UserIDContextKey)

	args := &protocol.CreateRoomArgs{}
	err := c.BindJSON(args)
	if err != nil {
		xl.Infof("invalid args in body, error %v", err)
		httpErr := errors.NewHTTPErrorBadRequest().WithRequestID(requestID).WithMessage("invalid args in request body")
		c.JSON(http.StatusBadRequest, httpErr)
		return
	}
	if !h.validateRoomName(args.RoomName) {
		xl.Infof("invalid room name %s", args.RoomName)
		httpErr := errors.NewHTTPErrorInvalidRoomName().WithRequestID(requestID).WithMessagef("invalid room name %s", args.RoomName)
		c.JSON(http.StatusBadRequest, httpErr)
		return
	}

	roomID := h.generateRoomID()
	room := &protocol.LiveRoom{
		ID:      roomID,
		Name:    args.RoomName,
		Creator: userID,
		PlayURL: h.generatePlayURL(roomID),
		RTCRoom: roomID,
		Status:  protocol.LiveRoomStatusSingle,
	}
	err = h.Room.CreateRoom(xl, room)
	if err != nil {
		serverErr, ok := err.(*errors.ServerError)
		if !ok {
			xl.Errorf("create room error %v", err)
			httpErr := errors.NewHTTPErrorInternal().WithRequestID(requestID)
			c.JSON(http.StatusInternalServerError, httpErr)
			return
		}
		switch serverErr.Code {
		case errors.ServerErrorRoomNameUsed:
			httpErr := errors.NewHTTPErrorRoomNameused().WithRequestID(requestID)
			c.JSON(http.StatusConflict, httpErr)
			return
		case errors.ServerErrorTooManyRooms:
			httpErr := errors.NewHTTPErrorTooManyRooms().WithRequestID(requestID)
			c.JSON(http.StatusServiceUnavailable, httpErr)
			return
		default:
			httpErr := errors.NewHTTPErrorInternal().WithRequestID(requestID)
			c.JSON(http.StatusInternalServerError, httpErr)
			return
		}
	}

	xl.Infof("user %s created room: ID %s, name %s", userID, roomID, args.RoomName)
	resp := &protocol.CreateRoomResponse{
		RoomID:       roomID,
		RoomName:     args.RoomName,
		RTCRoom:      roomID,
		RTCRoomToken: h.generateRTCRoomToken(roomID),
	}
	c.JSON(http.StatusOK, resp)
}

// validateRoomName 校验直播间名称。
func (h *RoomHandler) validateRoomName(roomName string) bool {
	roomNameMaxLength := 100
	if len(roomName) == 0 || len(roomName) > roomNameMaxLength {
		return false
	}
	return true
}

// generateRoomID 生成直播间ID。
func (h *RoomHandler) generateRoomID() string {
	alphaNum := "0123456789abcdefghijklmnopqrstuvwxyz"
	roomID := ""
	idLength := 16
	for i := 0; i < idLength; i++ {
		index := rand.Intn(len(alphaNum))
		roomID = roomID + string(alphaNum[index])
	}
	return roomID
}

func (h *RoomHandler) generatePlayURL(roomID string) string {
	return "rtmp://" + h.LiveHost + "/" + h.LiveHub + "/" + roomID
}

// TODO:生成加入RTC房间的room token。
func (h *RoomHandler) generateRTCRoomToken(roomID string) string {
	return ""
}

// CloseRoom 关闭直播间。
func (h *RoomHandler) CloseRoom(c *gin.Context) {
	xl := c.MustGet(protocol.XLogKey).(*xlog.Logger)
	requestID := xl.ReqId
	userID := c.GetString(protocol.UserIDContextKey)

	args := &protocol.CloseRoomArgs{}
	err := c.BindJSON(args)
	if err != nil {
		xl.Infof("invalid args in body, error %v", err)
		httpErr := errors.NewHTTPErrorBadRequest().WithRequestID(requestID).WithMessage("invalid args in request body")
		c.JSON(http.StatusBadRequest, httpErr)
		return
	}

	err = h.Room.CloseRoom(xl, userID, args.RoomID)
	if err != nil {
		serverErr, ok := err.(*errors.ServerError)
		if !ok {
			xl.Errorf("close room error %v", err)
			httpErr := errors.NewHTTPErrorInternal().WithRequestID(requestID)
			c.JSON(http.StatusInternalServerError, httpErr)
			return
		}
		switch serverErr.Code {
		case errors.ServerErrorRoomNotFound:
			httpErr := errors.NewHTTPErrorNoSuchRoom().WithRequestID(requestID)
			c.JSON(http.StatusNotFound, httpErr)
			return
		default:
			httpErr := errors.NewHTTPErrorInternal().WithRequestID(requestID)
			c.JSON(http.StatusInternalServerError, httpErr)
			return
		}
	}
	xl.Infof("user %s closed room: ID %s", userID, args.RoomID)
	// return OK
}