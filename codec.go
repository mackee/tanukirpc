package tanukirpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
)

var (
	ErrRequestNotSupportedAtThisCodec = errors.New("request not supported at this codec")
	ErrRequestContinueDecode          = errors.New("request continue decode")
	DefaultCodecList                  = CodecList{&urlParamCodec{}, NewFormCodec(), NewJSONCodec(), &nopCodec{}}
)

type Codec interface {
	Name() string
	Decode(r *http.Request, v any) error
	Encode(w http.ResponseWriter, r *http.Request, v any) error
}

type Decoder interface {
	Decode(v any) error
}

type DecoderFunc func(r io.Reader) Decoder

type Encoder interface {
	Encode(v any) error
}

type EncoderFunc func(w io.Writer) Encoder

const (
	defaultJSONCodecContentType = "application/json"
	defaultFormCodecContentType = "application/x-www-form-urlencoded"
)

func NewJSONCodec() *codec {
	return &codec{
		contentTypes:        []string{defaultJSONCodecContentType},
		acceptTypes:         []string{"*/*", defaultJSONCodecContentType},
		responseContentType: defaultJSONCodecContentType,
		decoderFunc: func(r io.Reader) Decoder {
			return json.NewDecoder(r)
		},
		encoderFunc: func(w io.Writer) Encoder {
			return json.NewEncoder(w)
		},
		name: "json",
	}
}

func NewFormCodec() *codec {
	return &codec{
		contentTypes:        []string{defaultFormCodecContentType},
		acceptTypes:         []string{},
		responseContentType: "",
		decoderFunc: func(r io.Reader) Decoder {
			return &renderDecoder{r: r, rd: render.DecodeForm}
		},
		encoderFunc: nil,
		name:        "form",
	}
}

type codec struct {
	contentTypes        []string
	acceptTypes         []string
	responseContentType string
	decoderFunc         DecoderFunc
	encoderFunc         EncoderFunc
	name                string
}

func (c *codec) Name() string {
	return c.name
}

func (c *codec) isMyContentType(contentType string) bool {
	return slices.Contains(c.contentTypes, contentType)
}

func (c *codec) Decode(r *http.Request, v any) error {
	if c.decoderFunc == nil {
		return ErrRequestNotSupportedAtThisCodec
	}

	if !c.isMyContentType(r.Header.Get("content-type")) {
		return ErrRequestNotSupportedAtThisCodec
	}

	if err := c.decoderFunc(r.Body).Decode(v); err != nil {
		return &ErrCodecDecode{err: err}
	}

	return nil
}

func (c *codec) Encode(w http.ResponseWriter, r *http.Request, v any) error {
	if c.encoderFunc == nil {
		return ErrRequestNotSupportedAtThisCodec
	}

	accept := r.Header.Get("accept")
	if !slices.Contains(c.acceptTypes, accept) {
		return ErrRequestNotSupportedAtThisCodec
	}

	w.Header().Set("content-type", c.responseContentType)
	if err := c.encoderFunc(w).Encode(v); err != nil {
		return &ErrCodecEncode{err: err}
	}

	return nil
}

type nopCodec struct{}

func (c *nopCodec) Name() string {
	return "nop"
}

func (c *nopCodec) Decode(r *http.Request, v any) error {
	if _, err := io.Copy(io.Discard, r.Body); err != nil {
		return &ErrCodecDecode{err: err}
	}
	return nil
}

func (c *nopCodec) Encode(w http.ResponseWriter, r *http.Request, v any) error {
	return nil
}

type ErrCodecDecode struct {
	err error
}

func (e *ErrCodecDecode) Error() string {
	return fmt.Sprintf("error decoding request: %v", e.err)
}

func (e *ErrCodecDecode) Unwrap() error {
	return e.err
}

type ErrCodecEncode struct {
	err error
}

func (e *ErrCodecEncode) Error() string {
	return fmt.Sprintf("error encoding response: %v", e.err)
}

func (e *ErrCodecEncode) Unwrap() error {
	return e.err
}

type CodecList []Codec

func (c CodecList) Name() string {
	return "list"
}

func (c CodecList) Decode(r *http.Request, v any) error {
	for _, codec := range c {
		if err := codec.Decode(r, v); err == nil {
			break
		} else if errors.Is(err, ErrRequestNotSupportedAtThisCodec) || errors.Is(err, ErrRequestContinueDecode) {
			continue
		} else {
			return fmt.Errorf("decode error in CodecList: %w, codec=%s", err, codec.Name())
		}
	}
	return nil
}

func (c CodecList) Encode(w http.ResponseWriter, r *http.Request, v any) error {
	for _, codec := range c {
		if err := codec.Encode(w, r, v); err == nil {
			break
		} else if errors.Is(err, ErrRequestNotSupportedAtThisCodec) {
			continue
		} else {
			return fmt.Errorf("encode error in CodecList: %w, codec=%s", err, codec.Name())
		}
	}
	return nil
}

type urlParamCodec struct{}

func (c *urlParamCodec) Name() string {
	return "urlparam"
}

func (c *urlParamCodec) Decode(r *http.Request, v any) error {
	vr := reflect.ValueOf(v)
	if vr.Kind() == reflect.Pointer {
		vr = vr.Elem()
	}
	if vr.Kind() != reflect.Struct {
		return errors.New("v must be a pointer to a struct")
	}
	str := vr.Type()
	for i := 0; i < vr.NumField(); i++ {
		ft := str.Field(i)
		field := vr.Field(i)
		if ft.Type.Kind() == reflect.Struct {
			if err := c.Decode(r, field.Interface()); err != nil {
				return fmt.Errorf("failed to decode field %s: %w", ft.Name, err)
			}
			continue
		}
		param := ft.Tag.Get("urlparam")
		if param == "" {
			continue
		}
		paramValue := chi.URLParam(r, param)
		if paramValue == "" {
			return fmt.Errorf("url param %s is required at field %s", param, ft.Name)
		}
		switch field.Kind() {
		case reflect.String:
			field.SetString(paramValue)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			pi, err := strconv.ParseInt(paramValue, 10, 64)
			if err != nil {
				return fmt.Errorf("failed to parse int at field %s: %w", ft.Name, err)
			}
			field.SetInt(pi)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			pu, err := strconv.ParseUint(paramValue, 10, 64)
			if err != nil {
				return fmt.Errorf("failed to parse uint at field %s: %w", ft.Name, err)
			}
			field.SetUint(pu)
		case reflect.Float32, reflect.Float64:
			pf, err := strconv.ParseFloat(paramValue, 64)
			if err != nil {
				return fmt.Errorf("failed to parse float at field %s: %w", ft.Name, err)
			}
			field.SetFloat(pf)
		case reflect.Complex64, reflect.Complex128:
			pc, err := strconv.ParseComplex(paramValue, 128)
			if err != nil {
				return fmt.Errorf("failed to parse complex at field %s: %w", ft.Name, err)
			}
			field.SetComplex(pc)
		case reflect.Bool:
			pb, err := strconv.ParseBool(paramValue)
			if err != nil {
				return fmt.Errorf("failed to parse bool at field %s: %w", ft.Name, err)
			}
			field.SetBool(pb)
		default:
			return fmt.Errorf("unsupported type at field %s: %s", ft.Name, field.Kind())
		}
	}

	return ErrRequestContinueDecode
}

func (c *urlParamCodec) Encode(w http.ResponseWriter, r *http.Request, v any) error {
	return ErrRequestNotSupportedAtThisCodec
}

type renderDecoder struct {
	r  io.Reader
	rd func(r io.Reader, req any) error
}

func (r *renderDecoder) Decode(v any) error {
	return r.rd(r.r, v)
}
