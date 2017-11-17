// buff project main.go
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Luxurioust/excelize"
)

var (
	inputFile  *string
	outputDir  *string
	tagBase    = [2]string{"<base>", "</base>"}
	tagStructs = [2]string{"<structs>", "</structs>"}
	tagStruct  = [2]string{"<struct>", "</struct>"}
	tagProtos  = [2]string{"<protos>", "</protos>"}
	tagProto   = [2]string{"<proto>", "</proto>"}
	tagReq     = [2]string{"<req>", "</req>"}
	tagRes     = [2]string{"<res>", "</res>"}

	sysTypes = map[string]string{
		"long":     "int64",
		"int":      "int32",
		"short":    "int16",
		"byte":     "byte",
		"double":   "float64",
		"string":   "string",
		"[]long":   "[]int64",
		"[]int":    "[]int32",
		"[]short":  "[]int16",
		"[]byte":   "[]byte",
		"[]double": "[]float64",
		"[]string": "[]string",
	}

	outputFile  string
	packName    string
	structDatas []*StructData
)

type StructData struct {
	name   string
	fields [][]string
}

func (p StructData) String() string {
	return fmt.Sprintf("name:%s fields:%v", p.name, p.fields)
}

func init() {
	inputFile = flag.String("i", "", "input excel file name")
	outputDir = flag.String("o", ".", "go file output dir")
	structDatas = make([]*StructData, 0)
}

func findtext(tag [2]string, rows [][]string) map[string][][]string {
	tagStart, tagEnd := tag[0], tag[1]
	datas := make(map[string][][]string, 0)
	for index := 0; index < len(rows); index++ {
		row := rows[index]
		//找到开始节点
		if row[0] == tagStart {
			name := row[1]
			startIndex := index
			fixdata := make([][]string, 0)
			var findEnd bool
			for index = index + 1; index < len(rows); index++ {
				row = rows[index]
				if row[0] == tagEnd {
					findEnd = true
					break
				} else if row[0] != "" {
					fixdata = append(fixdata, row)
				}
			}
			if !findEnd {
				panic(fmt.Sprintf("标签 %s[行号：%d] 没有找到匹配的结束标签 %s\n", tagStart, startIndex+1, tagEnd))
			}
			datas[name] = fixdata
		}
	}
	return datas
}

func parseBase(base [][]string) {
	for _, v := range base {
		switch v[0] {
		case "package":
			packName = v[1]
		case "output":
			outputFile = v[1]
		}
	}
}

func decodeDataType(sType string) string {
	dType, ok := sysTypes[sType]
	if ok {
		return dType
	}
	return sType
}

func parseStruct(name string, rows [][]string) {
	newCls := &StructData{}
	newCls.name = name
	newCls.fields = make([][]string, 0)
	structDatas = append(structDatas, newCls)

	for i := 0; i < len(rows); i++ {
		row := rows[i]
		fieldName, fieldType := row[0], row[1]
		fieldName = fmt.Sprintf("%s%s", strings.ToUpper(string([]rune(fieldName)[0])), string([]rune(fieldName)[1:]))
		switch fieldType {
		case "":
			newCls.fields = append(newCls.fields, []string{fieldName, ""})
		case "[]":
			prefix := row[0] + "["
			j := i + 1
			subrows := make([][]string, 0)
			for ; j < len(rows); j++ {
				subRow := rows[j]
				if !strings.HasPrefix(subRow[0], prefix) {
					break
				} else {
					text := []rune(strings.TrimPrefix(subRow[0], prefix))
					subFieldName := string(text[0 : len(text)-1])
					subrows = append(subrows, []string{subFieldName, subRow[1]})
				}
			}
			subName := name + fmt.Sprintf("%s%s", strings.ToUpper(string([]rune(row[0])[0])), string([]rune(row[0])[1:]))
			parseStruct(subName, subrows)
			newCls.fields = append(newCls.fields, []string{fieldName, "[]*" + subName})
			i = j - 1
		default:
			newCls.fields = append(newCls.fields, []string{fieldName, decodeDataType(fieldType)})
		}
	}
}

func printStruct(sd *StructData) []string {
	data := make([]string, 0)
	output := func(format string, params ...interface{}) {
		data = append(data, fmt.Sprintf(format, params...))
	}

	output("type %s struct {", sd.name)
	for _, field := range sd.fields {
		fieldName := field[0]
		fieldType := field[1]
		output("%s %s", fieldName, fieldType)
	}
	output("}")

	// func (t *XXX) Encode(writer *bytes.Buffer){
	//}
	output("func (p *%s) Encode(writer *bytes.Buffer) {", sd.name)
	for _, field := range sd.fields {
		fieldName := field[0]
		fieldType := field[1]
		switch fieldType {
		case "":
			output("p.%s.Encode(writer)", fieldName)
		case "int64", "int32", "int16", "byte", "float64":
			output("binary.Write(writer, binary.LittleEndian, p.%s)", fieldName)
		case "[]int64", "[]int32", "[]int16", "[]byte", "[]float64":
			output("binary.Write(writer, binary.LittleEndian, int16(len(p.%s)))", fieldName)
			output("for _, v := range p.%s {", fieldName)
			output("binary.Write(writer, binary.LittleEndian, v)")
			output("}")
		case "string":
			output("binary.Write(writer, binary.LittleEndian, int32(len(p.%s)))", fieldName)
			output("writer.WriteString(p.%s)", fieldName)
		case "[]string":
			output("binary.Write(writer, binary.LittleEndian, int16(len(p.%s)))", fieldName)
			output("for _, v := range p.%s {", fieldName)
			output("binary.Write(writer, binary.LittleEndian, int32(len(v)))")
			output("writer.WriteString(v)")
			output("}")
		default:
			output("binary.Write(writer, binary.LittleEndian, int16(len(p.%s)))", fieldName)
			output("for _, v := range p.%s {", fieldName)
			output("v.Encode(writer)")
			output("}")
		}
	}
	output("}")

	output("func (p *%s) Decode(reader *bytes.Buffer) {", sd.name)
	var defCount bool
	for _, field := range sd.fields {
		fieldName := field[0]
		fieldType := field[1]
		switch fieldType {
		case "":
			output("p.%s.Decode(reader)", fieldName)
		case "int64", "int32", "int16", "byte", "bool":
			output("binary.Read(reader, binary.LittleEndian, &p.%s)", fieldName)
		case "[]int64", "[]int32", "[]int16", "[]byte", "[]float64":
			if !defCount {
				defCount = true
				output("var count int16")
			}
			output("binary.Read(reader, binary.LittleEndian, &count)")
			output("p.%s = make(%s,count)", fieldName, fieldType)
			output("for i := int16(0); i < count; i++ {")
			output("binary.Read(reader, binary.LittleEndian, &p.%s[i])", fieldName)
			output("}")
		case "string":
			output("p.%s = coder.Decodestring(reader)", fieldName)
		case "[]string":
			if !defCount {
				defCount = true
				output("var count int16")
			}
			output("binary.Read(reader, binary.LittleEndian, &count)")
			output("p.%s = make([]string,count)", fieldName)
			output("for i := int16(0); i < count; i++ {")
			output("p.%s[i] = coder.Decodestring(reader)", fieldName)
			output("}")
		default:
			if !defCount {
				defCount = true
				output("var count int16")
			}
			output("binary.Read(reader, binary.LittleEndian, &count)")
			output("p.%s = make(%s, count)", fieldName, fieldType)
			output("for i := int16(0); i < count; i++ {")
			output("p.%s[i] = &%s{}", fieldName, strings.TrimPrefix(fieldType, "[]*"))
			output("p.%s[i].Decode(reader)", fieldName)
			output("}")
		}
	}
	output("}")

	return data
}

func handle() {
	xlsx, err := excelize.OpenFile(*inputFile)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	sheets := xlsx.GetSheetMap()
	sheetNames := make([]string, 0)
	for _, name := range sheets {
		rows := xlsx.GetRows(name)
		bases := findtext(tagBase, rows)
		if len(bases) == 0 {
			continue
		}

		sheetNames = append(sheetNames, name)
		fmt.Printf("%d:%-15s ", len(sheetNames), name)
		if len(sheetNames)%5 == 0 {
			fmt.Println()
		}
	}
	fmt.Println()

	var sheetIndex int

	for {
		fmt.Printf("请输入需解析的sheet索引:")
		fmt.Scanf("%d", &sheetIndex)
		if sheetIndex <= 0 || sheetIndex > len(sheetNames) {
			fmt.Print("索引输入错误，")
		} else {
			break
		}
	}

	rows := xlsx.GetRows(sheetNames[sheetIndex-1])
	bases := findtext(tagBase, rows)
	if len(bases) == 0 {
		fmt.Printf("没有找到标签 %s \n", tagBase[0])
		return
	}
	for _, v := range bases {
		parseBase(v)
	}

	for _, v := range findtext(tagStructs, rows) {
		for name, vv := range findtext(tagStruct, v) {
			parseStruct(name, vv[1:])
		}
	}

	for _, v := range findtext(tagProtos, rows) {
		for name, vv := range findtext(tagProto, v) {
			for _, vvv := range findtext(tagReq, vv) {
				parseStruct(name+"Req", vvv[1:])
			}
			for _, vvv := range findtext(tagRes, vv) {
				parseStruct(name+"Res", vvv[1:])
			}
		}
	}

	data := []string{"package " + packName, "import (", "\"bytes\"", "\"encoding/binary\"", "\"github.com/sencydai/excel2proto/coder\"", ")"}

	for _, v := range structDatas {
		data = append(data, printStruct(v)...)
	}

	if err = os.MkdirAll(*outputDir, 0777); err != nil {
		fmt.Printf("创建目录 %s 失败: %s\n", *outputDir, err.Error())
		return
	}

	absPath, _ := filepath.Abs(*outputDir)

	file, err := os.Create(absPath + "/" + outputFile)
	if err != nil {
		fmt.Printf("创建文件 %s 失败: %s\n", outputFile, err.Error())
		return
	}
	file.WriteString(strings.Join(data, "\n"))
	file.Close()

	tempFileName := absPath + "\\" + "temp_gofmt_file.bat"
	file, _ = os.Create(tempFileName)
	file.WriteString("@echo on\n")
	file.WriteString("cd " + absPath + "\n")
	file.WriteString("gofmt -w .")
	file.WriteString("\nexit\n")
	file.Close()

	defer os.Remove(tempFileName)

	cmd := exec.Command("cmd", "/c", tempFileName)
	if err := cmd.Run(); err != nil {
		fmt.Printf("gmfmt error: %s \n", err.Error())
	}

	fmt.Printf("【%s】转换成功 ==> %s\n", sheets[sheetIndex], absPath+"\\"+outputFile)
}

func main() {
	flag.Parse()
	handle()
}
