package deleteuserfile

type Request struct {
	OwnerUUID string `json:"_"`
	FileUUID  string `json:"uuid"`
}
