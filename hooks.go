package dalgo

// Property hold property name, value & flag to index
type Property struct {
	Name    string
	Value   interface{}
	NoIndex bool
}

// EntityLoader loads properties to struct
type EntityLoader interface {
	Load([]Property) error
}

// EntitySaver dumps struct to properties
type EntitySaver interface {
	Save() ([]Property, error)
}

//func SaveStruct(src interface{}) ([]Property, error) {
//	return nil, nil
//}
