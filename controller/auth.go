package controller

import (
	"context"

	"github.com/qiniu/qmgo"
	"github.com/qiniu/x/xlog"

	"github.com/qrtc/qlive/protocol"
)

// AuthController 控制用户认证。
type AuthController struct {
	mongoClient    *qmgo.Client
	activeUserColl *qmgo.Collection
	xl             *xlog.Logger
}

// NewAuthController 创建AuthController.
func NewAuthController(mongoURI string, database string, xl *xlog.Logger) (*AuthController, error) {
	if xl == nil {
		xl = xlog.New("qlive-account-controller")
	}
	mongoClient, err := qmgo.NewClient(context.Background(), &qmgo.Config{
		Uri:      mongoURI,
		Database: database,
	})
	if err != nil {
		xl.Errorf("failed to create mongo client, error %v", err)
		return nil, err
	}
	activeUserColl := mongoClient.Database(database).Collection(ActiveUserCollection)
	return &AuthController{
		mongoClient:    mongoClient,
		activeUserColl: activeUserColl,
		xl:             xl,
	}, nil
}

// GetIDByToken 根据token获取账号ID。如果未在已登录用户表查找到这个token，说明该token不合法。
func (c *AuthController) GetIDByToken(xl *xlog.Logger, token string) (id string, err error) {
	if xl == nil {
		xl = c.xl
	}
	activeUserRecord := &protocol.ActiveUser{}
	err = c.activeUserColl.Find(context.Background(), map[string]interface{}{"token": token}).One(activeUserRecord)
	if err != nil {
		if !qmgo.IsErrNoDocuments(err) {
			xl.Infof("token %s not found in active users", token)
			return "", err
		}
		xl.Errorf("failed to find token in active users, error %v", err)
		return "", err
	}
	return activeUserRecord.ID, nil
}