package controller

import (
	"fmt"
	"hash"
	"hash/fnv"
	"strconv"

	"github.com/davecgh/go-spew/spew"
	"k8s.io/apimachinery/pkg/util/rand"
)

//nolint:godox
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

	if _, err := printer.Fprintf(hasher, "%#v", objectToWrite); err != nil {
		return fmt.Errorf("failed to hash object: %w", err)
	}

	return nil
}

// ComputeHash returns a hash value calculated from the provided object.
// The hash will be safe encoded to avoid bad words.
func ComputeHash(object interface{}) (string, error) {
	hasher := fnv.New32a()
	if err := DeepHashObject(hasher, object); err != nil {
		return "", err
	}

	return rand.SafeEncodeString(strconv.FormatUint(uint64(hasher.Sum32()), 10)), nil
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
