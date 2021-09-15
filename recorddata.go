package dalgo

type RecordData struct {
	Properties map[string]interface{}
}

func (v RecordData) Validate() error {
	return nil
}
