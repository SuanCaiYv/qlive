package handler

import (
	"math/rand"
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/qiniu/x/xlog"

	"github.com/qrtc/qlive/errors"
	"github.com/qrtc/qlive/protocol"
)

// AccountInterface 获取账号信息的接口。
type AccountInterface interface {
	GetAccountByPhoneNumber(xl *xlog.Logger, phoneNumber string) (*protocol.Account, error)
	GetAccountByID(xl *xlog.Logger, id string) (*protocol.Account, error)
	CreateAccount(xl *xlog.Logger, account *protocol.Account) error
	UpdateAccount(xl *xlog.Logger, id string, account *protocol.Account) (*protocol.Account, error)
	AccountLogin(xl *xlog.Logger, id string) (token string, err error)
	AccountLogout(xl *xlog.Logger, id string) error
}

// SMSCodeInterface 发送短信验证码并记录的接口。
type SMSCodeInterface interface {
	Send(xl *xlog.Logger, phoneNumber string) (err error)
	Validate(xl *xlog.Logger, phoneNumber string, smsCode string) (err error)
}

// AccountHandler 处理与账号相关的请求：登录、注册、退出、修改账号信息等
type AccountHandler struct {
	Account AccountInterface
	SMSCode SMSCodeInterface
}

// validatePhoneNumber 检查手机号码是否符合规则。
func validatePhoneNumber(phoneNumber string) bool {
	phoneNumberRegExp := regexp.MustCompile(`1[3-9][0-9]{9}`)
	return phoneNumberRegExp.MatchString(phoneNumber)
}

// SendSMSCode 发送短信验证码。
func (h *AccountHandler) SendSMSCode(c *gin.Context) {
	xl := c.MustGet(protocol.XLogKey).(*xlog.Logger)
	requestID := xl.ReqId
	phoneNumber, ok := c.GetQuery("phone_number")
	if !ok {
		httpErr := errors.NewHTTPErrorInvalidPhoneNumber().WithRequestID(requestID).WithMessage("empty phone number")
		c.JSON(http.StatusBadRequest, httpErr)
		return
	}
	if !validatePhoneNumber(phoneNumber) {
		httpErr := errors.NewHTTPErrorInvalidPhoneNumber().WithRequestID(requestID).WithMessage("invalid phone number")
		c.JSON(http.StatusBadRequest, httpErr)
		return
	}
	err := h.SMSCode.Send(xl, phoneNumber)
	if err != nil {
		serverErr, ok := err.(*errors.ServerError)
		if ok && serverErr.Code == errors.ServerErrorSMSSendTooFrequent {
			xl.Infof("SMS code has been sent to %s, cannot resend in short time", phoneNumber)
			httpErr := errors.NewHTTPErrorSMSSendTooFrequent().WithRequestID(requestID)
			c.JSON(http.StatusTooManyRequests, httpErr)
			return
		}
		xl.Errorf("failed to send sms code to phone number %s, error %v", phoneNumber, err)
		c.JSON(http.StatusInternalServerError, err)
		return
	}
	xl.Infof("SMS code sent to number %s", phoneNumber)
	c.JSON(http.StatusOK, "")
}

const (
	// LoginTypeSMSCode 使用短信验证码登录
	LoginTypeSMSCode = "smscode"
)

// Login 处理登录请求，根据query分不同类型处理。
func (h *AccountHandler) Login(c *gin.Context) {
	xl := c.MustGet(protocol.XLogKey).(*xlog.Logger)
	requestID := xl.ReqId
	loginType, ok := c.GetQuery("logintype")
	if !ok {
		httpErr := errors.NewHTTPErrorBadLoginType().WithRequestID(requestID).WithMessage("empty login type")
		c.JSON(http.StatusBadRequest, httpErr)
		return
	}
	switch loginType {
	case LoginTypeSMSCode:
		h.LoginBySMS(c)
	default:
		httpErr := errors.NewHTTPErrorBadLoginType().WithRequestID(requestID).WithMessagef("login type %s not supported", loginType)
		c.JSON(http.StatusBadRequest, httpErr)
	}
}

// generateUserID 生成新的用户ID。
func (h *AccountHandler) generateUserID() string {
	alphaNum := "0123456789abcdefghijklmnopqrstuvwxyz"
	idLength := 12
	id := ""
	for i := 0; i < idLength; i++ {
		index := rand.Intn(len(alphaNum))
		id = id + string(alphaNum[index])
	}
	return id
}

// LoginBySMS 使用手机短信验证码登录。
func (h *AccountHandler) LoginBySMS(c *gin.Context) {
	xl := c.MustGet(protocol.XLogKey).(*xlog.Logger)
	requestID := xl.ReqId
	args := protocol.SMSLoginArgs{}
	err := c.BindJSON(&args)
	if err != nil {
		xl.Infof("invalid args in body, error %v", err)
		httpError := errors.NewHTTPErrorBadRequest().WithRequestID(requestID).WithMessage("invalid args in request body")
		c.JSON(http.StatusBadRequest, httpError)
		return
	}

	err = h.SMSCode.Validate(xl, args.PhoneNumber, args.SMSCode)
	if err != nil {
		xl.Infof("validate SMS code failed, error %v", err)
		httpErr := errors.NewHTTPErrorWrongSMSCode().WithRequestID(requestID)
		c.JSON(http.StatusUnauthorized, httpErr)
		return
	}
	account, err := h.Account.GetAccountByPhoneNumber(xl, args.PhoneNumber)
	if err != nil {
		if err.Error() == "not found" {
			xl.Infof("phone number %s not found, create new account", args.PhoneNumber)
			newAccount := &protocol.Account{
				ID:          h.generateUserID(),
				PhoneNumber: args.PhoneNumber,
			}
			createErr := h.Account.CreateAccount(xl, newAccount)
			if createErr != nil {
				xl.Errorf("failed to craete account, error %v", err)
				httpErr := errors.NewHTTPErrorUnauthorized().WithRequestID(requestID)
				c.JSON(http.StatusUnauthorized, httpErr)
				return
			}
			account = newAccount
		} else {
			xl.Errorf("get account by phone number failed, error %v", err)
			httpErr := errors.NewHTTPErrorUnauthorized().WithRequestID(requestID)
			c.JSON(http.StatusUnauthorized, httpErr)
			return
		}
	}
	// 更新该账号状态为已登录。
	token, err := h.Account.AccountLogin(xl, account.ID)
	if err != nil {
		serverErr, ok := err.(*errors.ServerError)
		if ok && serverErr.Code == errors.ServerErrorUserLoggedin {
			xl.Infof("user %s already logged in", account.ID)
			httpErr := errors.NewHTTPErrorAlreadyLoggedin().WithRequestID(requestID)
			c.JSON(http.StatusUnauthorized, httpErr)
			return
		}
		xl.Errorf("failed to set account %s to status logged in, error %v", account.ID, err)
		httpErr := errors.NewHTTPErrorUnauthorized().WithRequestID(requestID)
		c.JSON(http.StatusUnauthorized, httpErr)
		return
	}

	res := &protocol.LoginResponse{
		UserInfo: protocol.UserInfo{
			ID:       account.ID,
			Nickname: account.Nickname,
			Gender:   account.Gender,
		},
		Token: token,
	}
	c.SetCookie(protocol.LoginTokenKey, token, 0, "/", "qlive.qiniu.com", true, false)
	c.JSON(http.StatusOK, res)
}

// UpdateProfile 修改用户信息。
func (h *AccountHandler) UpdateProfile(c *gin.Context) {
	xl := c.MustGet(protocol.XLogKey).(*xlog.Logger)
	requestID := xl.ReqId
	id := c.GetString(protocol.UserIDContextKey)

	args := protocol.UpdateProfileArgs{}
	bindErr := c.BindJSON(&args)
	if bindErr != nil {
		xl.Infof("invalid args in request body, error %v", bindErr)
		httpErr := errors.NewHTTPErrorBadRequest().WithRequestID(requestID).WithMessage("invalid args in request body")
		c.JSON(http.StatusBadRequest, httpErr)
		return
	}

	account, err := h.Account.GetAccountByID(xl, id)
	if err != nil {
		xl.Infof("cannot find account, error %v", err)
		httpErr := errors.NewHTTPErrorNoSuchUser().WithRequestID(requestID).WithMessagef("user %s not found", id)
		c.JSON(http.StatusNotFound, httpErr)
		return
	}
	if account.ID != "" && account.ID != id {
		xl.Infof("user %s try to update profile of other user %s", id, account.ID)
		httpErr := errors.NewHTTPErrorNoSuchUser().WithRequestID(requestID).WithMessagef("user %s not found", id)
		c.JSON(http.StatusNotFound, httpErr)
		return
	}

	// TODO: validate updated profile.
	if args.Nickname != "" {
		account.Nickname = args.Nickname
	}
	if args.Gender != "" {
		account.Gender = args.Gender
	}

	newAccount, err := h.Account.UpdateAccount(xl, id, account)
	if err != nil {
		httpErr := errors.NewHTTPErrorInternal().WithRequestID(requestID).WithMessagef("update account failed: %v", err)
		c.JSON(http.StatusInternalServerError, httpErr)
		return
	}
	ret := &protocol.UpdateProfileResponse{
		ID:       newAccount.ID,
		Nickname: newAccount.Nickname,
		Gender:   newAccount.Gender,
	}
	c.JSON(http.StatusOK, ret)
}

// Logout 退出登录。
func (h *AccountHandler) Logout(c *gin.Context) {
	xl := c.MustGet(protocol.XLogKey).(*xlog.Logger)
	requestID := xl.ReqId
	id, exist := c.Get(protocol.UserIDContextKey)
	if !exist {
		xl.Infof("cannot find ID in context")
		httpErr := errors.NewHTTPErrorNotLoggedIn().WithRequestID(requestID)
		c.JSON(http.StatusUnauthorized, httpErr)
	}
	err := h.Account.AccountLogout(xl, id.(string))
	if err != nil {
		xl.Errorf("user %s log out error: %v", id, err)
		c.JSON(http.StatusUnauthorized, "")
	}
	xl.Infof("user %s logged out", id)
	c.SetCookie(protocol.LoginTokenKey, "", -1, "/", "qlive.qiniu.com", true, false)
	c.JSON(http.StatusOK, "")
}