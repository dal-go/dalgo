package end2end

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/end2end/models"
	"github.com/dal-go/dalgo/mocks/mock_dal"
	"github.com/dal-go/record"
	"go.uber.org/mock/gomock"
)

func TestEndToEnd_panics(t *testing.T) {
	t.Run("panics_on_nil_t", func(t *testing.T) {
		defer func() {
			if err := recover(); err == nil {
				t.Fatal("should panic on nil t parameters")
			}
		}()
		ctrl := gomock.NewController(t)
		db := mock_dal.NewMockDB(ctrl)
		TestDalgoDB(nil, db, nil, true)
	})
	t.Run("panics_on_nil_db", func(t *testing.T) {
		defer func() {
			if err := recover(); err == nil {
				t.Fatal("should panic on nil db parameters")
			}
		}()
		TestDalgoDB(t, nil, nil, true)
	})
}

func TestEndToEnd(t *testing.T) {
	dbCtrl := gomock.NewController(t)
	defer dbCtrl.Finish()

	var controllers []*gomock.Controller

	db := mock_dal.NewMockDB(dbCtrl)

	keyOnlyRecord := func(collection, id string) record.Record {
		return record.NewRecord(record.NewKeyWithID(collection, record.EscapeID(id)))
	}

	var getNumber int
	db.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, r record.Record) error {
		getNumber++
		switch getNumber {
		case 1:
			r.SetError(record.ErrRecordNotFound)
			return record.ErrRecordNotFound
		case 2:
			r.SetError(nil)
			data := r.Data().(*TestData)
			data.StringProp = "str1"
			data.IntegerProp = 1
			r.SetError(nil)
		}
		return nil
	}).Times(2)

	var existsCallNumber int
	db.EXPECT().Exists(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, key *record.Key) (bool, error) {
		existsCallNumber++
		switch existsCallNumber {
		case 1:
			return false, nil
		case 2:
			return true, nil
		case 3:
			return false, nil
		default:
			panic("unexpected call number")
		}
	}).Times(3)

	readCityIDs := func(cityIDs []string) func(ctx context.Context, query dal.Query) (dal.Reader, error) {
		return func(ctx context.Context, query dal.Query) (dal.Reader, error) {
			ctrl := gomock.NewController(t)
			controllers = append(controllers, ctrl)
			reader := mock_dal.NewMockRecordsReader(ctrl)
			i := 0
			sortedCityIDs := make([]string, len(cityIDs))
			copy(sortedCityIDs, cityIDs)
			switch q := query.(type) {
			case dal.StructuredQuery:
				if orderBy := q.OrderBy(); len(orderBy) == 1 {
					if orderBy[0].Descending() {
						slices.Reverse(sortedCityIDs)
					}
				}
				limit := query.Limit()
				if citiesCount := len(sortedCityIDs); limit == 0 || limit > citiesCount {
					limit = citiesCount
				}
				reader.EXPECT().Next().DoAndReturn(func() (r record.Record, err error) {
					if i >= limit {
						return nil, dal.ErrNoMoreRecords
					}
					r = keyOnlyRecord(models.CitiesCollection, sortedCityIDs[i])
					i++
					return
				}).AnyTimes()
				reader.EXPECT().Close().Times(1)
				return reader, nil
			default:
				return nil, fmt.Errorf("unexpected query type: %T", query)

			}
		}
	}

	// Expectation for calls WITHOUT transaction options
	db.EXPECT().RunReadwriteTransaction(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f dal.RWTxWorker, options ...dal.TransactionOption) error {
		ctrl := gomock.NewController(t)
		controllers = append(controllers, ctrl)
		tx := mock_dal.NewMockReadwriteTransaction(ctrl)

		txOptions := dal.NewTransactionOptions(options...)
		//tx.EXPECT().Options().Return(txOptions)

		txName := txOptions.Message()
		//t.Log("RW tx:", txName)
		switch txName {
		case "SELECT * FROM Cities: no_limit":
			tx.EXPECT().GetRecordsReader(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, query dal.Query) (reader dal.RecordsReader, err error) {
				records := make([]record.Record, len(models.Cities))
				for i, city := range models.Cities {
					key := record.NewKeyWithID("c1", city)
					records[i] = record.NewRecordWithData(key, &city)
				}
				return dal.NewRecordsReader(records), nil
			})
		case "SELECT * FROM Cities: limit=3":
			tx.EXPECT().GetRecordsReader(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, query dal.Query) (reader dal.RecordsReader, err error) {
				records := make([]record.Record, 3)
				for i, cityID := range models.SortedCityIDs[:3] {
					key := record.NewKeyWithID("c1", cityID)
					for _, city := range models.Cities {
						if models.CityID(city) == cityID {
							records[i] = record.NewRecordWithData(key, &city)
							break
						}
					}
				}
				return dal.NewRecordsReader(records), nil
			})
		case "singleDeleteTest":
			tx.EXPECT().Delete(ctx, gomock.Any()).Return(nil).Times(1)
		case "deleteAllRecords":
			tx.EXPECT().DeleteMulti(ctx, gomock.Any()).Return(nil).Times(1)
		case "deleteAllCities":
			tx.EXPECT().DeleteMulti(ctx, gomock.Any()).Return(nil).Times(1)
			tx.EXPECT().GetRecordsReader(gomock.Any(), gomock.Any()).DoAndReturn(readCityIDs(models.SortedCityIDs))
		case "singleCreateWithPredefinedIDTest":
			tx.EXPECT().Insert(ctx, gomock.Any()).Return(nil).Times(1)
		case "setMulti":
			tx.EXPECT().SetMulti(ctx, gomock.Any()).Return(nil).Times(1)
		case "update2records":
			tx.EXPECT().UpdateMulti(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
		case "setupDataForQueryTests":
			tx.EXPECT().SetMulti(ctx, gomock.Any()).Return(nil).Times(1)
		case "":
			panic("no RW tx name")
		default:
			panic("unexpected RW tx name: " + txName)
		}
		return f(ctx, tx)
	}).Times(13)

	db.EXPECT().RunReadonlyTransaction(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f dal.ROTxWorker, options ...dal.TransactionOption) error {
		ctrl := gomock.NewController(t)
		controllers = append(controllers, ctrl)
		tx := mock_dal.NewMockReadTransaction(ctrl)

		txOptions := dal.NewTransactionOptions(options...)
		//tx.EXPECT().Options().Return(txOptions)

		txName := txOptions.Message()
		//t.Log("RO tx:", txName)
		switch txName {
		case "SELECT ID FROM Cities; limit=0":
			tx.EXPECT().GetRecordsReader(gomock.Any(), gomock.Any()).DoAndReturn(readCityIDs(models.SortedCityIDs))
		case "SELECT ID FROM Cities ORDER BY Population; limit=3":
			tx.EXPECT().GetRecordsReader(gomock.Any(), gomock.Any()).DoAndReturn(readCityIDs(models.CityIDsSortedByPopulation))
		case "SELECT ID FROM Cities ORDER BY Population DESCENDING; limit=3":
			tx.EXPECT().GetRecordsReader(gomock.Any(), gomock.Any()).DoAndReturn(readCityIDs(models.CityIDsSortedByPopulation))
		case "SELECT ID FROM Cities WHERE Country = 'IN'":
			tx.EXPECT().GetRecordsReader(gomock.Any(), gomock.Any()).DoAndReturn(readCityIDs([]string{"Delhi_Delhi", "Maharashtra_Mumbai"}))
		case "SELECT Name AS city, Country FROM Cities",
			"SELECT Country, COUNT(*), SUM(Population) GROUP BY Country",
			"SELECT Country, COUNT(*) GROUP BY Country HAVING COUNT(*) > 1":
			// This mock DB does not implement column projection / GROUP BY, so
			// both read paths report the capability as unsupported and the
			// shared end2end subtests skip.
			tx.EXPECT().GetRecordsReader(gomock.Any(), gomock.Any()).Return(nil, dal.ErrNotSupported).AnyTimes()
			tx.EXPECT().GetRecordsetReader(gomock.Any(), gomock.Any()).Return(nil, dal.ErrNotSupported).AnyTimes()
		case "verify_cleanupDelete":
			tx.EXPECT().GetMulti(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, records []record.Record) error {
				for _, rec := range records {
					rec.SetError(record.ErrRecordNotFound)
				}
				return nil
			}).Times(1)
			//tx.EXPECT().QueryReader(gomock.Any(), gomock.Any()).DoAndReturn(readCityIDs(models.SortedCityIDs))
		case "get3NonExistingRecords":
			tx.EXPECT().GetMulti(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, records []record.Record) error {
				for _, rec := range records {
					rec.SetError(record.ErrRecordNotFound)
				}
				return nil
			}).Times(1)
		case "using_records_with_data":
			tx.EXPECT().GetMulti(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, records []record.Record) error {
				for _, record := range records {
					record.SetError(nil)
					data := record.Data().(*TestData)
					data.StringProp = record.Key().ID.(string) + "str"
				}
				return nil
			}).Times(1)
		case "getMulti2existing2missingRecords":
			tx.EXPECT().GetMulti(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, records []record.Record) error {
				r1 := records[0]
				r1.SetError(nil)
				d1 := r1.Data().(*TestData)
				d1.StringProp = "k1r1str"

				r2 := records[1]
				r2.SetError(nil)
				d2 := r2.Data().(*TestData)
				d2.StringProp = "k1r2str"

				r3 := records[2]
				r3.SetError(record.ErrRecordNotFound)

				r4 := records[3]
				r4.SetError(record.ErrRecordNotFound)

				return nil
			}).Times(1)
		case "getMultiNewRecords":
			tx.EXPECT().GetMulti(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, records []record.Record) error {
				//t.Log("len(records):", len(records))
				r1 := records[0]
				r1.SetError(nil)
				d1 := r1.Data().(*TestData)
				d1.StringProp = "UpdateD"

				r2 := records[1]
				r2.SetError(nil)
				d2 := r2.Data().(*TestData)
				d2.StringProp = "UpdateD"

				r3 := records[2]
				r3.SetError(nil)
				d3 := r3.Data().(*TestData)
				d3.StringProp = "k2r1str"

				return nil
			}).Times(1)
		case "selectAllCities":
			tx.EXPECT().GetRecordsReader(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, _ dal.Query) (dal.RecordsReader, error) {
				records := make([]record.Record, len(models.Cities))
				for i, city := range models.Cities {
					key := record.NewKeyWithID("c1", city)
					records[i] = record.NewRecordWithData(key, &city)
				}
				return dal.NewRecordsReader(records), nil
			}).Times(1)
		case "SELECT ID FROM Cities; limit=3":
			tx.EXPECT().GetRecordsReader(gomock.Any(), gomock.Any()).DoAndReturn(readCityIDs(models.SortedCityIDs))
		case "":
			panic("no RO tx name")
		default:
			panic("unexpected RO tx name: " + txName)
		}
		return f(ctx, tx)
	}).AnyTimes()

	TestDalgoDB(t, db, nil, true)

	for _, ctrl := range controllers {
		ctrl.Finish()
	}
}
