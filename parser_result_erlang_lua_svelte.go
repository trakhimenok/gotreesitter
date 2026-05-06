package gotreesitter

import "bytes"

func normalizeErlangSourceFileForms(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "erlang" || root.Type(lang) != "source_file" {
		return
	}
	formsOnlyID := FieldID(0)
	for i, fieldName := range lang.FieldNames {
		if fieldName == "forms_only" {
			formsOnlyID = FieldID(i)
			break
		}
	}
	if formsOnlyID == 0 || !erlangSourceFileLooksLikeForms(root, lang) {
		return
	}
	ensureNodeFieldStorage(root, len(root.children))
	for i, child := range root.children {
		if child == nil || child.IsExtra() {
			continue
		}
		root.fieldIDs[i] = formsOnlyID
		root.fieldSources[i] = fieldSourceDirect
		normalizeErlangTopLevelFormBounds(child)
	}
}

func erlangSourceFileLooksLikeForms(root *Node, lang *Language) bool {
	sawForm := false
	for _, child := range root.children {
		if child == nil || child.IsExtra() {
			continue
		}
		if !erlangIsTopLevelFormType(child.Type(lang)) {
			return false
		}
		sawForm = true
	}
	return sawForm
}

func erlangIsTopLevelFormType(typ string) bool {
	switch typ {
	case "module_attribute",
		"behaviour_attribute",
		"export_attribute",
		"import_attribute",
		"export_type_attribute",
		"optional_callbacks_attribute",
		"compile_options_attribute",
		"feature_attribute",
		"file_attribute",
		"deprecated_attribute",
		"record_decl",
		"type_alias",
		"nominal",
		"opaque",
		"spec",
		"callback",
		"wild_attribute",
		"fun_decl",
		"pp_include",
		"pp_include_lib",
		"pp_undef",
		"pp_ifdef",
		"pp_ifndef",
		"pp_else",
		"pp_endif",
		"pp_if",
		"pp_elif",
		"pp_define",
		"ssr_definition",
		"shebang":
		return true
	default:
		return false
	}
}

func normalizeErlangTopLevelFormBounds(node *Node) {
	if node == nil || len(node.children) == 0 {
		return
	}
	var first, last *Node
	for _, child := range node.children {
		if child == nil || child.IsExtra() {
			continue
		}
		if first == nil {
			first = child
		}
		last = child
	}
	if first == nil || last == nil {
		return
	}
	node.startByte = first.startByte
	node.startPoint = first.startPoint
	node.endByte = last.endByte
	node.endPoint = last.endPoint
}

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

func normalizeSvelteTrailingExtraTrivia(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "svelte" || root.Type(lang) != "document" || len(root.children) == 0 || len(source) == 0 {
		return
	}
	last := root.children[len(root.children)-1]
	if last == nil || last.IsNamed() || !last.IsExtra() || len(last.children) != 0 {
		return
	}
	if last.Type(lang) != "_tag_value_token1" {
		return
	}
	if last.startByte >= last.endByte || last.endByte != root.endByte || int(last.endByte) > len(source) {
		return
	}
	if !bytesAreTrivia(source[last.startByte:last.endByte]) {
		return
	}
	root.children = root.children[:len(root.children)-1]
	if len(root.fieldIDs) > len(root.children) {
		root.fieldIDs = root.fieldIDs[:len(root.children)]
	}
	if len(root.fieldSources) > len(root.children) {
		root.fieldSources = root.fieldSources[:len(root.children)]
	}
}
