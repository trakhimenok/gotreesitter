package gotreesitter

import "bytes"

func normalizeLuaChunkLocalDeclarationFields(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "lua" || root.Type(lang) != "chunk" || len(source) == 0 {
		return
	}
	localDeclID := FieldID(0)
	for i, fieldName := range lang.FieldNames {
		if fieldName == "local_declaration" {
			localDeclID = FieldID(i)
			break
		}
	}
	if localDeclID == 0 {
		return
	}
	ensureNodeFieldStorage(root, len(root.children))
	for i, child := range root.children {
		if child == nil || child.IsExtra() {
			continue
		}
		switch child.Type(lang) {
		case "function_declaration", "variable_declaration":
		default:
			continue
		}
		if !luaNodeStartsWithLocalKeyword(child, source) {
			continue
		}
		root.fieldIDs[i] = localDeclID
		root.fieldSources[i] = fieldSourceDirect
	}
}

func luaNodeStartsWithLocalKeyword(node *Node, source []byte) bool {
	if node == nil || node.startByte >= uint32(len(source)) {
		return false
	}
	start := int(node.startByte)
	if !bytes.HasPrefix(source[start:], []byte("local")) {
		return false
	}
	after := start + len("local")
	return after >= len(source) || source[after] == ' ' || source[after] == '\t' || source[after] == '\n' || source[after] == '\r'
}
