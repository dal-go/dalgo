package dal

type ClientInfo interface {
	Driver() string
	Version() string
}

func NewClientInfo(driver, version string) ClientInfo {
	return clientInfo{driver: driver, version: version}
}

var _ ClientInfo = (*clientInfo)(nil)

type clientInfo struct {
	driver  string
	version string
}

func (v clientInfo) Equals(other ClientInfo) bool {
	return v.driver == other.Driver() && v.version == other.Version()
}

func (v clientInfo) String() string {
	return v.driver + "@" + v.version
}

func (v clientInfo) Driver() string {
	return v.driver
}

func (v clientInfo) Version() string {
	return v.version
}
