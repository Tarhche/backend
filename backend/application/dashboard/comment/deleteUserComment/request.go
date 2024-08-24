package deleteUserComment

type Request struct {
	CommentUUID string `json:"uuid"`
	UserUUID    string `json:"_"`
}
