package gomodel

import (
	"github.com/zzl/go-winmd/apimodel"
	"path/filepath"
	"strings"
)

type ApiFilter struct {
	Namespaces    []string
	Architectures []string
	DllImports    []string
	MaxOsVersion  string
}

func (this *ApiFilter) IncludeNs(ns *apimodel.Namespace) bool {
	if this == nil || len(this.Namespaces) == 0 || ns == nil {
		return true
	}
	var include bool
	for _, filterNs := range this.Namespaces {
		var negative bool
		if filterNs[0] == '!' {
			negative = true
			filterNs = filterNs[1:]
		}
		match, _ := filepath.Match(filterNs, ns.FullName)
		if match {
			include = !negative
		}
	}
	return include
}

func (this *ApiFilter) IncludeDll(dll string) bool {
	if this == nil || len(this.DllImports) == 0 {
		return true
	}
	for _, dllImport := range this.DllImports {
		match := strings.EqualFold(dllImport, dll)
		if match {
			return true
		}
	}
	return false
}
