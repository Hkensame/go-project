// Code generated by goctl. DO NOT EDIT.
// goctl 1.7.3

package types

type LoginReq struct {
	Mobile   string `json:"mobile"`
	Password string `json:"password"`
}

type LoginRes struct {
	Token  string `json:"token"`
	Expire int64  `json:"expire"`
}

type RegisterReq struct {
	Mobile   string `json:"mobile"`
	Password string `json:"password"`
	UserName string `json:"username"`
	Gender   string `json:"gender"`
	Avatar   string `json:"avatar"`
}

type RegisterRes struct {
	Token  string `json:"token"`
	Expire int64  `json:"expire"`
}

type UserInfoReq struct {
}

type UserInfoRes struct {
	Id       string `json:"id"`
	Mobile   string `json:"mobile"`
	UserName string `json:"username"`
	Gender   string `json:"gender"`
	Avatar   string `json:"avatar"`
}
