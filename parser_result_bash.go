package gotreesitter

func normalizeBashProgramVariableAssignments(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "bash" || root.Type(lang) != "program" || len(root.children) == 0 {
		return
	}
	normalizeBashVariableAssignmentsInNode(root, lang)
}

func normalizeBashVariableAssignmentsInNode(node *Node, lang *Language) {
	if node == nil || lang == nil || len(node.children) == 0 {
		return
	}
	for _, child := range node.children {
		if child != nil {
			normalizeBashVariableAssignmentsInNode(child, lang)
		}
	}
	out := make([]*Node, 0, len(node.children))
	changed := false
	for _, child := range node.children {
		if child == nil {
			continue
		}
		if child.Type(lang) == "variable_assignments" && bashAllVariableAssignments(child, lang) && bashShouldSplitVariableAssignments(node.Type(lang)) {
			out = append(out, child.children...)
			changed = true
			continue
		}
		out = append(out, child)
	}
	if !changed {
		assignBashIfConditionField(node, lang)
		return
	}
	if node.ownerArena != nil {
		buf := node.ownerArena.allocNodeSlice(len(out))
		copy(buf, out)
		out = buf
	}
	node.children = out
	node.fieldIDs = nil
	node.fieldSources = nil
	assignBashIfConditionField(node, lang)
}

func bashAllVariableAssignments(node *Node, lang *Language) bool {
	if node == nil || lang == nil || len(node.children) < 2 {
		return false
	}
	for _, child := range node.children {
		if child == nil || child.Type(lang) != "variable_assignment" {
			return false
		}
	}
	return true
}

func bashShouldSplitVariableAssignments(parentType string) bool {
	switch parentType {
	case "command", "redirected_statement", "declaration_command", "unset_command":
		return false
	default:
		return true
	}
}

func assignBashIfConditionField(node *Node, lang *Language) {
	if node == nil || lang == nil || node.Type(lang) != "if_statement" || len(node.children) <= 1 {
		return
	}
	fid, ok := lang.FieldByName("condition")
	if !ok {
		return
	}
	ensureNodeFieldStorage(node, len(node.children))
	thenIndex := -1
	for i, child := range node.children {
		if child != nil && child.Type(lang) == "then" {
			thenIndex = i
			break
		}
	}
	if thenIndex < 0 {
		thenIndex = len(node.children)
	}
	for i := 1; i < thenIndex; i++ {
		if node.children[i] == nil {
			continue
		}
		node.fieldIDs[i] = fid
		node.fieldSources[i] = fieldSourceDirect
	}
}
