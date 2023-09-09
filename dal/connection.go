package dal

//import "context"
//
//// Connection represents a unique session to a data source.
//// Some drivers do not have/need this concept at low level.
//// Some like Go's standard `sql` manage pool of connections automatically.
//// We might want to mark it as obsolete or remove.
//type Connection interface {
//
//	// Close session, e.g. close connection or release any locks
//	// TODO: Needs use case, or an example how can be avoided (maybe with context cancellation?)
//	// TODO: Needs tests in dalgo-end2-end-tests to verify no operations are performed after connection closed
//	Close(ctx context.Context) error
//}
