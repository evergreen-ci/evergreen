package mongowire

import "strings"

func NamespaceIsCommand(ns string) bool {
	idx := strings.IndexByte(ns, '.')
	if idx == -1 || len(ns) <= idx+1 {
		return false
	}

	return ns[idx+1] == '$'
}

func NamespaceToDB(ns string) string {
	idx := strings.IndexByte(ns, '.')
	if idx == -1 {
		return ns
	}

	return ns[0:idx]
}

func NamespaceToCollection(ns string) string {
	idx := strings.IndexByte(ns, '.')
	if idx == -1 {
		return ""
	}

	return ns[idx+1:]
}
