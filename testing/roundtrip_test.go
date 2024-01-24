package testing

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"
	"time"

	"github.com/ipfs/go-cid"

	"github.com/google/go-cmp/cmp"

	cbg "github.com/whyrusleeping/cbor-gen"
)

var alwaysEqual = cmp.Comparer(func(_, _ interface{}) bool { return true })

// This option handles slices and maps of any type.
var alwaysEqualOpt = cmp.FilterValues(func(x, y interface{}) bool {
	vx, vy := reflect.ValueOf(x), reflect.ValueOf(y)
	return (vx.IsValid() && vy.IsValid() && vx.Type() == vy.Type()) &&
		(vx.Kind() == reflect.Slice || vx.Kind() == reflect.Map) &&
		(vx.Len() == 0 && vy.Len() == 0)
}, alwaysEqual)

func TestSimpleSigned(t *testing.T) {
	testTypeRoundtrips(t, reflect.TypeOf(SignedArray{}))
}

func TestSimpleTypeOne(t *testing.T) {
	testTypeRoundtrips(t, reflect.TypeOf(SimpleTypeOne{}))
}

func TestSimpleTypeTwo(t *testing.T) {
	testTypeRoundtrips(t, reflect.TypeOf(SimpleTypeTwo{}))
}

func TestSimpleTypeTree(t *testing.T) {
	testTypeRoundtrips(t, reflect.TypeOf(SimpleTypeTree{}))
}

func TestNeedScratchForMap(t *testing.T) {
	testTypeRoundtrips(t, reflect.TypeOf(NeedScratchForMap{}))
}

func TestNilPreserveWorks(t *testing.T) {
	testTypeRoundtrips(t, reflect.TypeOf(TestSliceNilPreserve{}))
}

func testValueRoundtrip(t *testing.T, obj cbg.CBORMarshaler, nobj cbg.CBORUnmarshaler) {
	t.Helper()

	buf := new(bytes.Buffer)
	if err := obj.MarshalCBOR(buf); err != nil {
		t.Fatal("i guess its fine to fail marshaling")
	}

	enc := buf.Bytes()

	if err := nobj.UnmarshalCBOR(bytes.NewReader(enc)); err != nil {
		t.Logf("got bad bytes: %x", enc)
		t.Fatal("failed to round trip object: ", err)
	}

	if !cmp.Equal(obj, nobj, alwaysEqualOpt) {
		t.Logf("%#v != %#v", obj, nobj)
		t.Log("not equal after round trip!")
	}

	nbuf := new(bytes.Buffer)
	if err := nobj.(cbg.CBORMarshaler).MarshalCBOR(nbuf); err != nil {
		t.Fatal("failed to remarshal object: ", err)
	}

	if !bytes.Equal(nbuf.Bytes(), enc) {
		fmt.Printf("%#v\n", obj)
		fmt.Printf("%#v\n", nobj)
		t.Fatalf("objects encodings different: %x != %x", nbuf.Bytes(), enc)
	}
}

func testTypeRoundtrips(t *testing.T, typ reflect.Type) {
	t.Helper()
	r := rand.New(rand.NewSource(56887))
	for i := 0; i < 1000; i++ {
		val, ok := quick.Value(typ, r)
		if !ok {
			t.Fatal("failed to generate test value")
		}

		obj := val.Addr().Interface().(cbg.CBORMarshaler)
		nobj := reflect.New(typ).Interface().(cbg.CBORUnmarshaler)
		testValueRoundtrip(t, obj, nobj)
	}
}

func TestDeferredContainer(t *testing.T) {
	zero := &DeferredContainer{}
	recepticle := &DeferredContainer{}
	testValueRoundtrip(t, zero, recepticle)
}

func TestNilValueDeferredUnmarshaling(t *testing.T) {
	var zero DeferredContainer
	zero.Deferred = &cbg.Deferred{Raw: []byte{0xf6}}

	buf := new(bytes.Buffer)
	if err := zero.MarshalCBOR(buf); err != nil {
		t.Fatal(err)
	}

	var n DeferredContainer
	if err := n.UnmarshalCBOR(buf); err != nil {
		t.Fatal(err)
	}

	if n.Deferred == nil {
		t.Fatal("shouldnt be nil!")
	}
}

func TestFixedArrays(t *testing.T) {
	zero := &FixedArrays{}
	recepticle := &FixedArrays{}
	testValueRoundtrip(t, zero, recepticle)
}

func TestTimeIsh(t *testing.T) {
	val := &ThingWithSomeTime{
		When:    cbg.CborTime(time.Now()),
		Stuff:   1234,
		CatName: "hank",
	}

	buf := new(bytes.Buffer)
	if err := val.MarshalCBOR(buf); err != nil {
		t.Fatal(err)
	}

	out := ThingWithSomeTime{}
	if err := out.UnmarshalCBOR(buf); err != nil {
		t.Fatal(err)
	}

	if out.When.Time().UnixNano() != val.When.Time().UnixNano() {
		t.Fatal("time didnt round trip properly", out.When.Time(), val.When.Time())
	}

	if out.Stuff != val.Stuff {
		t.Fatal("no")
	}

	if out.CatName != val.CatName {
		t.Fatal("no")
	}

	b, err := json.Marshal(val)
	if err != nil {
		t.Fatal(err)
	}

	var out2 ThingWithSomeTime
	if err := json.Unmarshal(b, &out2); err != nil {
		t.Fatal(err)
	}

	if !out.When.Time().Equal(out2.When.Time()) {
		t.Fatalf("when: got %#v, wanted %#v", out2.When, out.When)
	}
}

func TestLessToMoreFieldsRoundTrip(t *testing.T) {
	dummyCid, _ := cid.Parse("bafkqaaa")
	simpleTypeOne := SimpleTypeOne{
		Foo:     "foo",
		Value:   1,
		Binary:  []byte("bin"),
		Signed:  -1,
		NString: "namedstr",
		Strings: []string{"cat", "dog", "bear"},
	}
	obj := &SimpleStructV1{
		OldStr:    "hello",
		OldBytes:  []byte("bytes"),
		OldNum:    10,
		OldPtr:    &dummyCid,
		OldMap:    map[string]SimpleTypeOne{"first": simpleTypeOne},
		OldArray:  []SimpleTypeOne{simpleTypeOne},
		OldStruct: simpleTypeOne,
	}

	buf := new(bytes.Buffer)
	if err := obj.MarshalCBOR(buf); err != nil {
		t.Fatal("failed marshaling", err)
	}

	enc := buf.Bytes()

	nobj := SimpleStructV2{}
	if err := nobj.UnmarshalCBOR(bytes.NewReader(enc)); err != nil {
		t.Logf("got bad bytes: %x", enc)
		t.Fatal("failed to round trip object: ", err)
	}

	if obj.OldStr != nobj.OldStr {
		t.Fatal("mismatch ", obj.OldStr, " != ", nobj.OldStr)
	}
	if nobj.NewStr != "" {
		t.Fatal("expected field to be zero value")
	}

	if obj.OldNum != nobj.OldNum {
		t.Fatal("mismatch ", obj.OldNum, " != ", nobj.OldNum)
	}
	if nobj.NewNum != 0 {
		t.Fatal("expected field to be zero value")
	}

	if !bytes.Equal(obj.OldBytes, nobj.OldBytes) {
		t.Fatal("mismatch ", obj.OldBytes, " != ", nobj.OldBytes)
	}
	if nobj.NewBytes != nil {
		t.Fatal("expected field to be zero value")
	}

	if *obj.OldPtr != *nobj.OldPtr {
		t.Fatal("mismatch ", obj.OldPtr, " != ", nobj.OldPtr)
	}
	if nobj.NewPtr != nil {
		t.Fatal("expected field to be zero value")
	}

	if !cmp.Equal(obj.OldMap, nobj.OldMap) {
		t.Fatal("mismatch map marshal / unmarshal")
	}
	if len(nobj.NewMap) != 0 {
		t.Fatal("expected field to be zero value")
	}

	if !cmp.Equal(obj.OldArray, nobj.OldArray) {
		t.Fatal("mismatch array marshal / unmarshal")
	}
	if len(nobj.NewArray) != 0 {
		t.Fatal("expected field to be zero value")
	}

	if !cmp.Equal(obj.OldStruct, nobj.OldStruct) {
		t.Fatal("mismatch struct marshal / unmarshal")
	}
	if !cmp.Equal(nobj.NewStruct, SimpleTypeOne{}) {
		t.Fatal("expected field to be zero value")
	}
}

func TestMoreToLessFieldsRoundTrip(t *testing.T) {
	dummyCid1, _ := cid.Parse("bafkqaaa")
	dummyCid2, _ := cid.Parse("bafkqaab")
	simpleType1 := SimpleTypeOne{
		Foo:     "foo",
		Value:   1,
		Binary:  []byte("bin"),
		Signed:  -1,
		NString: "namedstr",
	}
	simpleType2 := SimpleTypeOne{
		Foo:     "bar",
		Value:   2,
		Binary:  []byte("bin2"),
		Signed:  -2,
		NString: "namedstr2",
	}
	obj := &SimpleStructV2{
		OldStr:    "oldstr",
		NewStr:    "newstr",
		OldBytes:  []byte("oldbytes"),
		NewBytes:  []byte("newbytes"),
		OldNum:    10,
		NewNum:    11,
		OldPtr:    &dummyCid1,
		NewPtr:    &dummyCid2,
		OldMap:    map[string]SimpleTypeOne{"foo": simpleType1},
		NewMap:    map[string]SimpleTypeOne{"bar": simpleType2},
		OldArray:  []SimpleTypeOne{simpleType1},
		NewArray:  []SimpleTypeOne{simpleType1, simpleType2},
		OldStruct: simpleType1,
		NewStruct: simpleType2,
	}

	buf := new(bytes.Buffer)
	if err := obj.MarshalCBOR(buf); err != nil {
		t.Fatal("failed marshaling", err)
	}

	enc := buf.Bytes()

	nobj := SimpleStructV1{}
	if err := nobj.UnmarshalCBOR(bytes.NewReader(enc)); err != nil {
		t.Logf("got bad bytes: %x", enc)
		t.Fatal("failed to round trip object: ", err)
	}

	if obj.OldStr != nobj.OldStr {
		t.Fatal("mismatch", obj.OldStr, " != ", nobj.OldStr)
	}
	if obj.OldNum != nobj.OldNum {
		t.Fatal("mismatch ", obj.OldNum, " != ", nobj.OldNum)
	}
	if !bytes.Equal(obj.OldBytes, nobj.OldBytes) {
		t.Fatal("mismatch ", obj.OldBytes, " != ", nobj.OldBytes)
	}
	if *obj.OldPtr != *nobj.OldPtr {
		t.Fatal("mismatch ", obj.OldPtr, " != ", nobj.OldPtr)
	}
	if !cmp.Equal(obj.OldMap, nobj.OldMap) {
		t.Fatal("mismatch map marshal / unmarshal")
	}
	if !cmp.Equal(obj.OldArray, nobj.OldArray) {
		t.Fatal("mismatch array marshal / unmarshal")
	}
	if !cmp.Equal(obj.OldStruct, nobj.OldStruct) {
		t.Fatal("mismatch struct marshal / unmarshal")
	}
}

func TestErrUnexpectedEOF(t *testing.T) {
	err := quick.Check(func(val SimpleTypeTwo, endIdx uint) bool {
		return t.Run("quickcheck", func(t *testing.T) {
			buf := new(bytes.Buffer)
			if err := val.MarshalCBOR(buf); err != nil {
				t.Error(err)
			}

			enc := buf.Bytes()
			originalLen := len(enc)
			endIdx = endIdx % uint(len(enc))
			enc = enc[:endIdx]

			nobj := SimpleTypeTwo{}
			err := nobj.UnmarshalCBOR(bytes.NewReader(enc))
			t.Logf("endIdx=%v, originalLen=%v", endIdx, originalLen)
			if int(endIdx) == originalLen && err != nil {
				t.Fatal("failed to round trip object: ", err)
			} else if endIdx == 0 && !errors.Is(err, io.EOF) {
				t.Fatal("expected EOF got", err)
			} else if endIdx != 0 && err == io.EOF {
				t.Fatal("did not expect EOF but got it")
			}
		})
	}, &quick.Config{MaxCount: 1000})
	if err != nil {
		t.Error(err)
	}
}

func TestLargeField(t *testing.T) {
	// 10 MB of data is the specified max so  4 MiB should work
	bs := make([]byte, 2<<21)
	bs[2<<20] = 0xaa // flags to check that serialization works
	bs[2<<20+2<<19] = 0xbb
	bs[2<<21-1] = 0xcc
	typ := BigField{
		LargeBytes: bs,
	}
	buf := new(bytes.Buffer)
	if err := typ.MarshalCBOR(buf); err != nil {
		t.Error(err)
	}
	enc := buf.Bytes()
	typ.LargeBytes = make([]byte, 0) // reset
	if err := typ.UnmarshalCBOR(bytes.NewReader(enc)); err != nil {
		t.Error(err)
	}

	// 16 MiB > 10, fails
	bs = make([]byte, 2<<23)
	badType := BigField{
		LargeBytes: bs,
	}
	buf = new(bytes.Buffer)
	err := badType.MarshalCBOR(buf)
	if err == nil {
		t.Fatal("buffer bigger than specified in struct tag should fail")
	}
}

func TestOmitEmpty(t *testing.T) {
	et := TestEmpty{
		Cat: 167,
	}

	recepticle := TestEmpty{}

	testValueRoundtrip(t, &et, &recepticle)
}

func TestConstRoundtrip(t *testing.T) {
	tcf := &TestConstField{
		Thing: 16223,
	}

	buf := new(bytes.Buffer)
	if err := tcf.MarshalCBOR(buf); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("%x\n", buf.Bytes())

	var out TestConstField
	if err := out.UnmarshalCBOR(buf); err != nil {
		t.Fatal(err)
	}

	fmt.Println(out)
}

func TestMapOfStringToString(t *testing.T) {
	mss := &MapStringString{Snorkleblump: map[string]string{
		"leave me":    "like this",
		"RAT":         "ATA",
		"Tears":       "eyes",
		"Rumble":      "killers in the jungle",
		"Butterflies": "caterpillars",
		"Inhale":      "Exhale",
		"A Street":    "I know",
		"XENA":        "ahhhhhhhhhh",
		"TOO":         "BIZARRE",
		"Stay":        "Hydrated",
		"Good":        "Space",
		"Super":       "sonic",
		"Hazel":       "theme",
		"Still":       "Here with the ones that I came with",
	}}

	buf := new(bytes.Buffer)
	if err := mss.MarshalCBOR(buf); err != nil {
		t.Fatal(err)
	}

	var out MapStringString
	if err := out.UnmarshalCBOR(buf); err != nil {
		t.Fatal(err)
	}

	fmt.Println(out)
}

//TODO same for strings

func TestTransparentIntArray(t *testing.T) {
	t.Run("roundtrip", func(t *testing.T) {
		zero := &IntArray{}
		recepticle := &IntArray{}
		testValueRoundtrip(t, zero, recepticle)
	})

	t.Run("roundtrip intalias", func(t *testing.T) {
		zero := &IntAliasArray{}
		recepticle := &IntAliasArray{}
		testValueRoundtrip(t, zero, recepticle)
	})

	// non-zero values
	t.Run("roundtrip non-zero", func(t *testing.T) {
		val := &IntArray{Ints: []int64{1, 2, 3}}
		recepticle := &IntArray{}
		testValueRoundtrip(t, val, recepticle)
	})
	t.Run("roundtrip non-zero intalias", func(t *testing.T) {
		val := &IntAliasArray{Ints: []IntAlias{1, 2, 3}}
		recepticle := &IntAliasArray{}
		testValueRoundtrip(t, val, recepticle)
	})

	// tuple struct to/from transparent int array
	t.Run("roundtrip tuple struct to transparent", func(t *testing.T) {
		val := &TupleIntArray{2, 4, 5}
		recepticle := &IntArray{}
		testValueRoundtrip(t, val, recepticle)
		if val.Int1 != recepticle.Ints[0] {
			t.Fatal("mismatch")
		}
	})
	t.Run("roundtrip transparent to tuple struct", func(t *testing.T) {
		val := &IntArray{Ints: []int64{2, 4, 5}}
		recepticle := &TupleIntArray{}
		testValueRoundtrip(t, val, recepticle)
		if val.Ints[0] != recepticle.Int1 {
			t.Fatal("mismatch")
		}
	})

	// IntArrayNewType / IntArrayAliasNewType
	t.Run("roundtrip IntArrayNewType", func(t *testing.T) {
		zero := &IntArrayNewType{}
		recepticle := &IntArrayNewType{}
		testValueRoundtrip(t, zero, recepticle)
	})
	t.Run("roundtrip IntArrayAliasNewType", func(t *testing.T) {
		zero := &IntArrayAliasNewType{}
		recepticle := &IntArrayAliasNewType{}
		testValueRoundtrip(t, zero, recepticle)
	})
	t.Run("roundtrip non-zero IntArrayNewType", func(t *testing.T) {
		val := &IntArrayNewType{1, 2, 3}
		recepticle := &IntArrayNewType{}
		testValueRoundtrip(t, val, recepticle)
	})
	t.Run("roundtrip non-zero IntArrayAliasNewType", func(t *testing.T) {
		val := &IntArrayAliasNewType{1, 2, 3}
		recepticle := &IntArrayAliasNewType{}
		testValueRoundtrip(t, val, recepticle)
	})
	// NewTypes into/from TupleIntArray
	t.Run("roundtrip IntArrayNewType to TupleIntArray", func(t *testing.T) {
		val := IntArrayNewType{1, 2, 3}
		recepticle := &TupleIntArray{}
		testValueRoundtrip(t, &val, recepticle)
		if val[0] != recepticle.Int1 {
			t.Fatal("mismatch")
		}
	})
	t.Run("roundtrip IntArrayAliasNewType to TupleIntArray", func(t *testing.T) {
		val := IntArrayAliasNewType{1, 2, 3}
		recepticle := &TupleIntArray{}
		testValueRoundtrip(t, &val, recepticle)
		if int64(val[0]) != recepticle.Int1 {
			t.Fatal("mismatch")
		}
	})
	t.Run("roundtrip TupleIntArray to IntArrayNewType", func(t *testing.T) {
		val := TupleIntArray{2, 4, 5}
		recepticle := IntArrayNewType{}
		testValueRoundtrip(t, &val, &recepticle)
		if val.Int1 != recepticle[0] {
			t.Fatal("mismatch")
		}
	})
	t.Run("roundtrip TupleIntArray to IntArrayAliasNewType", func(t *testing.T) {
		val := TupleIntArray{2, 4, 5}
		recepticle := IntArrayAliasNewType{}
		testValueRoundtrip(t, &val, &recepticle)
		if val.Int1 != int64(recepticle[0]) {
			t.Fatal("mismatch")
		}
	})
}

func TestMapTransparentType(t *testing.T) {
	t.Run("roundtrip", func(t *testing.T) {
		zero := MapTransparentType{}
		recepticle := &MapTransparentType{}
		testValueRoundtrip(t, &zero, recepticle)
	})

	// non-zero values
	t.Run("roundtrip non-zero", func(t *testing.T) {
		val := MapTransparentType(map[string]string{"foo": "bar"})
		recepticle := &MapTransparentType{}
		testValueRoundtrip(t, &val, recepticle)
	})
}
