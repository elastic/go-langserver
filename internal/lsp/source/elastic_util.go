package source

import (
	"go/types"
)

func (ident IdentifierInfo) GetDeclObject() types.Object {
	return ident.Declaration.obj
}
