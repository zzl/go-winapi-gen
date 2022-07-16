package utils

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func CapSafeName(name string) string {
	return CapName(SafeName(name))
}

func CapName(name string) string {
	var c uint8
	for {
		c = name[0]
		if c != '_' {
			break
		}
		name = name[1:] + "_"
	}
	if c >= 'a' && c <= 'z' {
		name = string(c-32) + name[1:]
	}
	//?
	//name = strings.Replace(name, "_e__Union", "", 1)
	//name = strings.Replace(name, "_e__Struct", "", 1)
	return name
}

func SafeName(name string) string {
	reservedNames := []string{"type", "var", "range", "map"}
	for _, it := range reservedNames {
		if name == it {
			return name + "_"
		}
	}
	return name
}

func BuildGuidExpr(sGuid string) string {
	expr := "syscall.GUID{0x" + sGuid[:8] +
		", 0x" + sGuid[9:13] + ", 0x" + sGuid[14:18] + ", \n\t[8]byte{"
	sGuid = strings.Replace(sGuid[19:], "-", "", 1)
	for n := 0; n < 16; n += 2 {
		if n > 0 {
			expr += ", "
		}
		expr += "0x" + sGuid[n:n+2]
	}
	expr += "}}"
	return expr
}

func CleanDir(dir string) {
	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Panic(err)
	}
	for _, fi := range fis {
		if fi.IsDir() {
			continue
		}
		c0 := fi.Name()[0]
		if c0 >= '0' && c0 <= '9' {
			continue
		}
		os.Remove(filepath.Join(dir, fi.Name()))
	}
}
