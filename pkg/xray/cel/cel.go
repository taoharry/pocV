package cel

import (
	"strings"

	"github.com/WAY29/pocV/internal/common/errors"

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

	c.envOptions = NewFunctionDefineOptions(reg)

	c.programOptions = NewFunctionImplOptions(reg)
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
