package mongodb

import (
	"errors"
	"strings"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"k8s.io/apimachinery/pkg/watch"
)

var (
	addedIndex = mgo.Index{
		Name:     "unique_resource_added",
		Key:      []string{"eventtype", "uid"},
		DropDups: true,
		PartialFilter: bson.M{
			"eventtype": watch.Added,
		},
		Unique: true,
	}

	deletedIndex = mgo.Index{
		Name:     "unique_resource_deleted",
		Key:      []string{"eventtype", "uid"},
		DropDups: true,
		PartialFilter: bson.M{
			"eventtype": watch.Deleted,
		},
		Unique: true,
	}
)

func (s *Storage) ensureIndexes() error {
	s.log.Debugf("Ensure indexes")
	var errs []string

	{
		collection := s.db.C(EventsCollection)
		if err := collection.EnsureIndex(addedIndex); err != nil {
			errs = append(errs, err.Error())
		}
		if err := collection.EnsureIndex(deletedIndex); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, ","))
	}

	return nil
}