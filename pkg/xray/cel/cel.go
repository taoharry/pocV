package cel

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/dlclark/regexp2"

	"github.com/WAY29/pocV/internal/common/errors"

	common_structs "github.com/WAY29/pocV/pkg/common/structs"
	"github.com/WAY29/pocV/pkg/xray/requests"
	"github.com/WAY29/pocV/pkg/xray/structs"
	"github.com/WAY29/pocV/utils"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/interpreter/functions"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
	"gopkg.in/yaml.v2"
)

// 自定义Lib库，包含变量和函数

type Env = cel.Env
type CustomLib struct {
	envOptions     []cel.EnvOption
	programOptions []cel.ProgramOption
}

// 执行表达式
func Evaluate(env *cel.Env, expression string, params map[string]interface{}) (ref.Val, error) {
	utils.DebugF("Evaluate expression: %s", strings.TrimSpace(expression))

	ast, iss := env.Compile(expression)
	err := iss.Err()
	if err != nil {
		wrappedErr := errors.Newf(errors.CompileError, "Compile error: %v", err)
		return nil, wrappedErr
	}

	prg, err := env.Program(ast)
	if err != nil {
		wrappedErr := errors.Newf(errors.ProgramCreationError, "Program creation error: %v", err)
		return nil, wrappedErr
	}

	out, _, err := prg.Eval(params)
	if err != nil {
		wrappedErr := errors.Newf(errors.EvaluationError, "Evaluation error: %v", err)
		return nil, wrappedErr
	}
	return out, nil
}

func UrlTypeToString(u *structs.UrlType) string {
	var buf strings.Builder
	if u.Scheme != "" {
		buf.WriteString(u.Scheme)
		buf.WriteByte(':')
	}
	if u.Scheme != "" || u.Host != "" {
		if u.Host != "" || u.Path != "" {
			buf.WriteString("//")
		}
		if h := u.Host; h != "" {
			buf.WriteString(u.Host)
		}
	}
	path := u.Path
	if path != "" && path[0] != '/' && u.Host != "" {
		buf.WriteByte('/')
	}
	if buf.Len() == 0 {
		if i := strings.IndexByte(path, ':'); i > -1 && strings.IndexByte(path[:i], '/') == -1 {
			buf.WriteString("./")
		}
	}
	buf.WriteString(path)

	if u.Query != "" {
		buf.WriteByte('?')
		buf.WriteString(u.Query)
	}
	if u.Fragment != "" {
		buf.WriteByte('#')
		buf.WriteString(u.Fragment)
	}
	return buf.String()
}

func NewEnv(c *CustomLib) (*cel.Env, error) {
	return cel.NewEnv(cel.Lib(c))
}

func NewEnvOption() CustomLib {
	c := CustomLib{}
	reg := types.NewEmptyRegistry()
	strStrMapType := decls.NewMapType(decls.String, decls.Bytes)

	c.envOptions = []cel.EnvOption{
		cel.CustomTypeAdapter(reg),
		cel.CustomTypeProvider(reg),
		cel.Container("structs"),
		cel.Types(
			&structs.UrlType{},
			&structs.Request{},
			&structs.Response{},
			&structs.Reverse{},
			strStrMapType,
		),
		cel.Declarations(
			decls.NewVar("request", decls.NewObjectType("structs.Request")),
			decls.NewVar("response", decls.NewObjectType("structs.Response")),
		),
		cel.Declarations(
			// functions
			decls.NewFunction("bcontains",
				decls.NewInstanceOverload("bytes_bcontains_bytes",
					[]*exprpb.Type{decls.Bytes, decls.Bytes},
					decls.Bool)),
			decls.NewFunction("ibcontains",
				decls.NewInstanceOverload("bytes_ibcontains_bytes",
					[]*exprpb.Type{decls.Bytes, decls.Bytes},
					decls.Bool)),
			decls.NewFunction("icontains",
				decls.NewInstanceOverload("icontains_string",
					[]*exprpb.Type{decls.String, decls.String},
					decls.Bool)),
			decls.NewFunction("bstartsWith",
				decls.NewInstanceOverload("bytes_bstartsWith_bytes",
					[]*exprpb.Type{decls.Bytes, decls.Bytes},
					decls.Bool)),
			decls.NewFunction("submatch",
				decls.NewInstanceOverload("string_submatch_string",
					[]*exprpb.Type{decls.String, decls.String},
					strStrMapType,
				)),
			decls.NewFunction("bmatches",
				decls.NewInstanceOverload("string_bmatches_bytes",
					[]*exprpb.Type{decls.String, decls.Bytes},
					decls.Bool)),
			decls.NewFunction("bsubmatch",
				decls.NewInstanceOverload("string_bsubmatch_bytes",
					[]*exprpb.Type{decls.String, decls.Bytes},
					strStrMapType,
				)),
			decls.NewFunction("wait",
				decls.NewInstanceOverload("reverse_wait_int",
					[]*exprpb.Type{decls.Any, decls.Int},
					decls.Bool)),
			decls.NewFunction("md5",
				decls.NewOverload("md5_string",
					[]*exprpb.Type{decls.String},
					decls.String)),
			decls.NewFunction("randomInt",
				decls.NewOverload("randomInt_int_int",
					[]*exprpb.Type{decls.Int, decls.Int},
					decls.Int)),
			decls.NewFunction("randomLowercase",
				decls.NewOverload("randomLowercase_int",
					[]*exprpb.Type{decls.Int},
					decls.String)),
			decls.NewFunction("base64",
				decls.NewOverload("base64_string",
					[]*exprpb.Type{decls.String},
					decls.String)),
			decls.NewFunction("base64",
				decls.NewOverload("base64_bytes",
					[]*exprpb.Type{decls.Bytes},
					decls.String)),
			decls.NewFunction("base64Decode",
				decls.NewOverload("base64Decode_string",
					[]*exprpb.Type{decls.String},
					decls.String)),
			decls.NewFunction("base64Decode",
				decls.NewOverload("base64Decode_bytes",
					[]*exprpb.Type{decls.Bytes},
					decls.String)),
			decls.NewFunction("urlencode",
				decls.NewOverload("urlencode_string",
					[]*exprpb.Type{decls.String},
					decls.String)),
			decls.NewFunction("urlencode",
				decls.NewOverload("urlencode_bytes",
					[]*exprpb.Type{decls.Bytes},
					decls.String)),
			decls.NewFunction("urldecode",
				decls.NewOverload("urldecode_string",
					[]*exprpb.Type{decls.String},
					decls.String)),
			decls.NewFunction("urldecode",
				decls.NewOverload("urldecode_bytes",
					[]*exprpb.Type{decls.Bytes},
					decls.String)),
			decls.NewFunction("substr",
				decls.NewOverload("substr_string_int_int",
					[]*exprpb.Type{decls.String, decls.Int, decls.Int},
					decls.String)),
			decls.NewFunction("replaceAll",
				decls.NewOverload("replaceAll_string_string_string",
					[]*exprpb.Type{decls.String, decls.String, decls.String},
					decls.String)),
			decls.NewFunction("printable",
				decls.NewOverload("printable_string",
					[]*exprpb.Type{decls.String},
					decls.String)),
			decls.NewFunction("sleep",
				decls.NewOverload("sleep_int",
					[]*exprpb.Type{decls.Int},
					decls.Bool)),
		),
	}
	c.programOptions = []cel.ProgramOption{
		cel.Functions(
			&functions.Overload{
				Operator: "bytes_bcontains_bytes",
				Binary: func(lhs ref.Val, rhs ref.Val) ref.Val {
					v1, ok := lhs.(types.Bytes)
					if !ok {
						return types.ValOrErr(lhs, "unexpected type '%v' passed to bcontains", lhs.Type())
					}
					v2, ok := rhs.(types.Bytes)
					if !ok {
						return types.ValOrErr(rhs, "unexpected type '%v' passed to bcontains", rhs.Type())
					}
					return types.Bool(bytes.Contains(v1, v2))
				},
			},
			&functions.Overload{
				Operator: "bytes_ibcontains_bytes",
				Binary: func(lhs ref.Val, rhs ref.Val) ref.Val {
					v1, ok := lhs.(types.Bytes)
					if !ok {
						return types.ValOrErr(lhs, "unexpected type '%v' passed to bcontains", lhs.Type())
					}
					v2, ok := rhs.(types.Bytes)
					if !ok {
						return types.ValOrErr(rhs, "unexpected type '%v' passed to bcontains", rhs.Type())
					}
					return types.Bool(bytes.Contains(bytes.ToLower(v1), bytes.ToLower(v2)))
				},
			},
			&functions.Overload{
				Operator: "icontains_string",
				Binary: func(lhs ref.Val, rhs ref.Val) ref.Val {
					v1, ok := lhs.(types.String)
					if !ok {
						return types.ValOrErr(lhs, "unexpected type '%v' passed to bcontains", lhs.Type())
					}
					v2, ok := rhs.(types.String)
					if !ok {
						return types.ValOrErr(rhs, "unexpected type '%v' passed to bcontains", rhs.Type())
					}
					// 不区分大小写包含
					return types.Bool(strings.Contains(strings.ToLower(string(v1)), strings.ToLower(string(v2))))
				},
			},
			&functions.Overload{
				Operator: "bytes_bstartsWith_bytes",
				Binary: func(lhs ref.Val, rhs ref.Val) ref.Val {
					v1, ok := lhs.(types.Bytes)
					if !ok {
						return types.ValOrErr(lhs, "unexpected type '%v' passed to bstartsWith", lhs.Type())
					}
					v2, ok := rhs.(types.Bytes)
					if !ok {
						return types.ValOrErr(rhs, "unexpected type '%v' passed to bstartsWith", rhs.Type())
					}
					return types.Bool(bytes.HasPrefix(v1, v2))
				},
			},
			&functions.Overload{
				Operator: "string_bmatches_bytes",
				Binary: func(lhs ref.Val, rhs ref.Val) ref.Val {
					var isMatch = false
					var err error

					v1, ok := lhs.(types.String)
					if !ok {
						return types.ValOrErr(lhs, "unexpected type '%v' passed to bmatches", lhs.Type())
					}
					v2, ok := rhs.(types.Bytes)
					if !ok {
						return types.ValOrErr(rhs, "unexpected type '%v' passed to bmatches", rhs.Type())
					}
					re := regexp2.MustCompile(string(v1), 0)
					if isMatch, err = re.MatchString(string([]byte(v2))); err != nil {
						return types.NewErr("%v", err)
					}
					return types.Bool(isMatch)
				},
			},
			&functions.Overload{
				Operator: "matches_string",
				Binary: func(lhs ref.Val, rhs ref.Val) ref.Val {
					var (
						isMatch = false
						err     error
					)

					v1, ok := lhs.(types.String)
					if !ok {
						return types.ValOrErr(lhs, "unexpected type '%v' passed to matches", lhs.Type())
					}
					v2, ok := rhs.(types.String)
					if !ok {
						return types.ValOrErr(rhs, "unexpected type '%v' passed to matches", rhs.Type())
					}

					re := regexp2.MustCompile(string(v1), 0)
					if isMatch, err = re.MatchString(string(v2)); err != nil {
						return types.NewErr("%v", err)
					}
					return types.Bool(isMatch)
				},
			},
			&functions.Overload{
				Operator: "string_submatch_string",
				Binary: func(lhs ref.Val, rhs ref.Val) ref.Val {
					var (
						resultMap = make(map[string]string)
					)

					v1, ok := lhs.(types.String)
					if !ok {
						return types.ValOrErr(lhs, "unexpected type '%v' passed to submatch", lhs.Type())
					}
					v2, ok := rhs.(types.String)
					if !ok {
						return types.ValOrErr(rhs, "unexpected type '%v' passed to submatch", rhs.Type())
					}

					re := regexp2.MustCompile(string(v1), regexp2.RE2)
					if m, _ := re.FindStringMatch(string(v2)); m != nil {
						gps := m.Groups()
						for n, gp := range gps {
							if n == 0 {
								continue
							}
							resultMap[gp.Name] = gp.String()
						}
					}
					return types.NewStringStringMap(reg, resultMap)
				},
			},
			&functions.Overload{
				Operator: "string_bsubmatch_bytes",
				Binary: func(lhs ref.Val, rhs ref.Val) ref.Val {
					var (
						resultMap = make(map[string]string)
					)

					v1, ok := lhs.(types.String)
					if !ok {
						return types.ValOrErr(lhs, "unexpected type '%v' passed to bsubmatch", lhs.Type())
					}
					v2, ok := rhs.(types.Bytes)
					if !ok {
						return types.ValOrErr(rhs, "unexpected type '%v' passed to bsubmatch", rhs.Type())
					}

					re := regexp2.MustCompile(string(v1), regexp2.RE2)
					if m, _ := re.FindStringMatch(string([]byte(v2))); m != nil {
						gps := m.Groups()
						for n, gp := range gps {
							if n == 0 {
								continue
							}
							resultMap[gp.Name] = gp.String()
						}
					}
					return types.NewStringStringMap(reg, resultMap)
				},
			},
			&functions.Overload{
				Operator: "md5_string",
				Unary: func(value ref.Val) ref.Val {
					v, ok := value.(types.String)
					if !ok {
						return types.ValOrErr(value, "unexpected type '%v' passed to md5_string", value.Type())
					}
					return types.String(fmt.Sprintf("%x", md5.Sum([]byte(v))))
				},
			},
			&functions.Overload{
				Operator: "randomInt_int_int",
				Binary: func(lhs ref.Val, rhs ref.Val) ref.Val {
					from, ok := lhs.(types.Int)
					if !ok {
						return types.ValOrErr(lhs, "unexpected type '%v' passed to randomInt", lhs.Type())
					}
					to, ok := rhs.(types.Int)
					if !ok {
						return types.ValOrErr(rhs, "unexpected type '%v' passed to randomInt", rhs.Type())
					}
					min, max := int(from), int(to)
					return types.Int(rand.Intn(max-min) + min)
				},
			},
			&functions.Overload{
				Operator: "randomLowercase_int",
				Unary: func(value ref.Val) ref.Val {
					n, ok := value.(types.Int)
					if !ok {
						return types.ValOrErr(value, "unexpected type '%v' passed to randomLowercase", value.Type())
					}
					return types.String(utils.RandomStr(utils.AsciiLowercase, int(n)))
				},
			},
			&functions.Overload{
				Operator: "base64_string",
				Unary: func(value ref.Val) ref.Val {
					v, ok := value.(types.String)
					if !ok {
						return types.ValOrErr(value, "unexpected type '%v' passed to base64_string", value.Type())
					}
					return types.String(base64.StdEncoding.EncodeToString([]byte(v)))
				},
			},
			&functions.Overload{
				Operator: "base64_bytes",
				Unary: func(value ref.Val) ref.Val {
					v, ok := value.(types.Bytes)
					if !ok {
						return types.ValOrErr(value, "unexpected type '%v' passed to base64_bytes", value.Type())
					}
					return types.String(base64.StdEncoding.EncodeToString(v))
				},
			},
			&functions.Overload{
				Operator: "base64Decode_string",
				Unary: func(value ref.Val) ref.Val {
					v, ok := value.(types.String)
					if !ok {
						return types.ValOrErr(value, "unexpected type '%v' passed to base64Decode_string", value.Type())
					}
					decodeBytes, err := base64.StdEncoding.DecodeString(string(v))
					if err != nil {
						return types.NewErr("%v", err)
					}
					return types.String(decodeBytes)
				},
			},
			&functions.Overload{
				Operator: "base64Decode_bytes",
				Unary: func(value ref.Val) ref.Val {
					v, ok := value.(types.Bytes)
					if !ok {
						return types.ValOrErr(value, "unexpected type '%v' passed to base64Decode_bytes", value.Type())
					}
					decodeBytes, err := base64.StdEncoding.DecodeString(string(v))
					if err != nil {
						return types.NewErr("%v", err)
					}
					return types.String(decodeBytes)
				},
			},
			&functions.Overload{
				Operator: "urlencode_string",
				Unary: func(value ref.Val) ref.Val {
					v, ok := value.(types.String)
					if !ok {
						return types.ValOrErr(value, "unexpected type '%v' passed to urlencode_string", value.Type())
					}
					return types.String(url.QueryEscape(string(v)))
				},
			},
			&functions.Overload{
				Operator: "urlencode_bytes",
				Unary: func(value ref.Val) ref.Val {
					v, ok := value.(types.Bytes)
					if !ok {
						return types.ValOrErr(value, "unexpected type '%v' passed to urlencode_bytes", value.Type())
					}
					return types.String(url.QueryEscape(string(v)))
				},
			},
			&functions.Overload{
				Operator: "urldecode_string",
				Unary: func(value ref.Val) ref.Val {
					v, ok := value.(types.String)
					if !ok {
						return types.ValOrErr(value, "unexpected type '%v' passed to urldecode_string", value.Type())
					}
					decodeString, err := url.QueryUnescape(string(v))
					if err != nil {
						return types.NewErr("%v", err)
					}
					return types.String(decodeString)
				},
			},
			&functions.Overload{
				Operator: "urldecode_bytes",
				Unary: func(value ref.Val) ref.Val {
					v, ok := value.(types.Bytes)
					if !ok {
						return types.ValOrErr(value, "unexpected type '%v' passed to urldecode_bytes", value.Type())
					}
					decodeString, err := url.QueryUnescape(string(v))
					if err != nil {
						return types.NewErr("%v", err)
					}
					return types.String(decodeString)
				},
			},
			&functions.Overload{
				Operator: "substr_string_int_int",
				Function: func(values ...ref.Val) ref.Val {
					if len(values) == 3 {
						str, ok := values[0].(types.String)
						if !ok {
							return types.NewErr("invalid string to 'substr'")
						}
						start, ok := values[1].(types.Int)
						if !ok {
							return types.NewErr("invalid start to 'substr'")
						}
						length, ok := values[2].(types.Int)
						if !ok {
							return types.NewErr("invalid length to 'substr'")
						}
						runes := []rune(str)
						if start < 0 || length < 0 || int(start+length) > len(runes) {
							return types.NewErr("invalid start or length to 'substr'")
						}
						return types.String(runes[start : start+length])
					} else {
						return types.NewErr("too many arguments to 'substr'")
					}
				},
			},
			&functions.Overload{
				Operator: "reverse_wait_int",
				Binary: func(lhs ref.Val, rhs ref.Val) ref.Val {
					reverse, ok := lhs.Value().(*structs.Reverse)
					if !ok {
						return types.ValOrErr(lhs, "unexpected type '%v' passed to 'wait'", lhs.Type())
					}
					timeout, ok := rhs.Value().(int64)
					if !ok {
						return types.ValOrErr(rhs, "unexpected type '%v' passed to 'wait'", rhs.Type())
					}

					return types.Bool(reverseCheck(reverse, timeout))
				},
			},
			&functions.Overload{
				Operator: "replaceAll_string_string_string",
				Function: func(values ...ref.Val) ref.Val {
					s, ok := values[0].(types.String)
					if !ok {
						return types.ValOrErr(s, "unexpected type '%v' passed to replaceAll", s.Type())
					}
					old, ok := values[1].(types.String)
					if !ok {
						return types.ValOrErr(old, "unexpected type '%v' passed to replaceAll", old.Type())
					}
					new, ok := values[2].(types.String)
					if !ok {
						return types.ValOrErr(new, "unexpected type '%v' passed to replaceAll", new.Type())
					}

					return types.String(strings.ReplaceAll(string(s), string(old), string(new)))
				},
			},
			&functions.Overload{
				Operator: "printable_string",
				Unary: func(value ref.Val) ref.Val {
					s, ok := value.(types.String)
					if !ok {
						return types.ValOrErr(s, "unexpected type '%v' passed to printable", s.Type())
					}

					clean := strings.Map(func(r rune) rune {
						if unicode.IsPrint(r) {
							return r
						}
						return -1
					}, string(s))

					return types.String(clean)
				},
			},
			&functions.Overload{
				Operator: "sleep_int",
				Unary: func(value ref.Val) ref.Val {
					i, ok := value.(types.Int)
					if !ok {
						return types.ValOrErr(i, "unexpected type '%v' passed to sleep", i.Type())
					}
					time.Sleep(time.Duration(int64(i)) * time.Second)
					return types.Bool(true)
				},
			},
		),
	}
	return c
}

// 声明环境中的变量类型和函数
func (c *CustomLib) CompileOptions() []cel.EnvOption {
	return c.envOptions
}

func (c *CustomLib) ProgramOptions() []cel.ProgramOption {
	return c.programOptions
}

func (c *CustomLib) UpdateCompileOptions(args yaml.MapSlice) {
	for _, item := range args {
		k, v := item.Key.(string), item.Value.(string)
		// 在执行之前是不知道变量的类型的，所以统一声明为字符型
		// 所以randomInt虽然返回的是int型，在运算中却被当作字符型进行计算，需要重载string_*_string
		var d *exprpb.Decl
		if strings.HasPrefix(v, "randomInt") {
			d = decls.NewVar(k, decls.Int)
		} else if strings.HasPrefix(v, "newReverse") {
			d = decls.NewVar(k, decls.NewObjectType("structs.Reverse"))
		} else {
			d = decls.NewVar(k, decls.String)
		}
		c.envOptions = append(c.envOptions, cel.Declarations(d))
	}
}

func (c *CustomLib) NewResultFunction(funcName string, returnBool bool) {
	c.envOptions = append(c.envOptions, cel.Declarations(
		decls.NewFunction(funcName,
			decls.NewOverload(funcName,
				[]*exprpb.Type{},
				decls.Bool)),
	),
	)

	c.programOptions = append(c.programOptions, cel.Functions(
		&functions.Overload{
			Operator: funcName,
			Function: func(values ...ref.Val) ref.Val {
				return types.Bool(returnBool)
			},
		}))

}

func reverseCheck(r *structs.Reverse, timeout int64) bool {
	switch r.ReverseType {
	case structs.ReverseType_Ceye:
		time.Sleep(time.Second * time.Duration(timeout))
		sub := strings.Split(r.Domain, ".")[0]
		urlStr := fmt.Sprintf("http://api.ceye.io/v1/records?token=%s&type=dns&filter=%s", common_structs.CeyeApi, sub)
		req, _ := http.NewRequest("GET", urlStr, nil)
		resp, _, err := requests.DoRequest(req, false)
		if err != nil {
			wrappedErr := errors.Wrap(err, "Reverse check error")
			utils.ErrorP(wrappedErr)
			return false
		}
		content, _ := requests.GetRespBody(resp)

		if !bytes.Contains(content, []byte(`"data": []`)) { // api返回结果不为空
			utils.DebugF("Got reverse dnslog from %s", r.Domain)
			return true
		}
		return false
	case structs.ReverseType_DnslogCN:
		time.Sleep(time.Second * time.Duration(timeout))
		sub := strings.Split(r.Domain, ".")[0]
		resp, _, err := requests.DoRequest(common_structs.DnslogCNGetRecordRequest, false)
		if err != nil {
			wrappedErr := errors.Wrap(err, "Reverse check error")
			utils.ErrorP(wrappedErr)
			return false
		}
		content, _ := requests.GetRespBody(resp)

		if bytes.Contains(content, []byte(sub)) { // api返回结果存在域名
			utils.DebugF("Got reverse dnslog from %s", r.Domain)
			return true
		}
		return false
	default:
		return false
	}

}
