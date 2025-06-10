package controller

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"hash"
	"hash/fnv"
	"k8s.io/apimachinery/pkg/util/rand"
)

// TODO: move to machinery

// DeepHashObject writes specified object to hash using the spew library
// which follows pointers and prints actual values of the nested objects
// ensuring the hash does not change when a pointer changes.
func DeepHashObject(hasher hash.Hash, objectToWrite interface{}) error {
	hasher.Reset()
	printer := spew.ConfigState{
		Indent:         " ",
		SortKeys:       true,
		DisableMethods: true,
		SpewKeys:       true,
	}

	_, err := printer.Fprintf(hasher, "%#v", objectToWrite)
	return err
}

// ComputeHash returns a hash value calculated from the provided object.
// The hash will be safe encoded to avoid bad words.
func ComputeHash(object interface{}) (string, error) {
	hasher := fnv.New32a()
	if err := DeepHashObject(hasher, object); err != nil {
		return "", err
	}

	return rand.SafeEncodeString(fmt.Sprint(hasher.Sum32())), nil
}

// ComputeVersionedHash follows the same rules of ComputeHash with the exception that accepts also a epoc value.
// The epoc value is used to generate a new hash from a same object.
// This is useful to force a new hash even if the original object is not changed.
// A practical use is to force a reconciliation loop of the object.
func ComputeVersionedHash(object interface{}, epoc int) (string, error) {
	type versionedHash struct {
		object interface{}
		epoc   int
	}

	return ComputeHash(versionedHash{object: object, epoc: epoc})
}
