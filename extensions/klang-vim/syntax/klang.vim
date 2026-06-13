if exists('b:current_syntax')
  finish
endif

syn case match

syn keyword klangKeyword if then else unless for while do do_while end return break continue try catch throw case partial import namespace region alias call function_group
syn keyword klangKeyword await defer private inline lazy async inner
syn keyword klangOperator and or xor not is as in move copy clone
syn keyword klangStorage global local let var val const mut export
syn keyword klangType Int UInt String Float Bool Char Map List Table Option Result Complex SIMD Function Awaitable Iterator Coroutine Atomic Any T Args Box Ref RefMut RefCell HeapAllocator RegionAllocator BumpAllocator ArenaAllocator
syn keyword klangConstant True False None Some Ok Err Result DEFAULT
syn keyword klangBuiltin print input len range iter next coroutine resume awaitable Async Atomic atomic_load atomic_store atomic_add sizeof get_default_procces_allocator free_all_allocator

syn match klangComment '--.*$'
syn match klangNumber '\v<\d+(\.\d+)?>'
syn match klangDirective '#\%(extend\|region\|sizeof\|call_site\|set_entry_point_to_here\)\>'
syn match klangHook '\[\%(new\|delete\|side_effects\)\]'
syn match klangAnnotation '@deprecated\%(([^)]*)\)\?'
syn match klangOperatorSymbol '\(|>\|==\|!=\|>=\|<=\|:=\|+=\|-=\|\*=\|/=\|\*\*\|->\|=>\|!!\|[=<>+\-*/%|!?]\)'

syn match klangFunctionDecl '\v<function>\s+\zs[A-Za-z_][A-Za-z0-9_]*'
syn match klangAliasFunctionDecl '\v<alias>\s+<function>\s+\zs[A-Za-z_][A-Za-z0-9_]*'
syn match klangNamespaceDecl '\v<namespace>\s+\zs[A-Za-z_][A-Za-z0-9_]*'
syn match klangTraitDecl '\v<trait>\s+\zs[A-Za-z_][A-Za-z0-9_]*'
syn match klangImplDecl '\v<impl>\s+\zs[A-Za-z_][A-Za-z0-9_]*'
syn match klangRegionDecl '\v<region>\s+\zs[A-Za-z_][A-Za-z0-9_]*'
syn match klangFunctionCall '\v<[A-Za-z_][A-Za-z0-9_]*\ze\s*\('

syn region klangString start=+"+ skip=+\\\\\|\\"+ end=+"+ contains=klangEscape
syn region klangChar start=+'+ skip=+\\\\\|\\'+ end=+'+ contains=klangEscape
syn region klangHereString start='^\s*//' end='^\s*//\s*;\?' keepend
syn match klangEscape '\\[\\''"nrt]' contained
syn match klangInvalidEscape '\\.' contained

hi def link klangKeyword Keyword
hi def link klangOperator Operator
hi def link klangStorage StorageClass
hi def link klangType Type
hi def link klangConstant Constant
hi def link klangBuiltin Function
hi def link klangComment Comment
hi def link klangNumber Number
hi def link klangDirective PreProc
hi def link klangHook PreProc
hi def link klangAnnotation PreProc
hi def link klangOperatorSymbol Operator
hi def link klangFunctionDecl Function
hi def link klangAliasFunctionDecl Type
hi def link klangNamespaceDecl Identifier
hi def link klangTraitDecl Type
hi def link klangImplDecl Type
hi def link klangRegionDecl Identifier
hi def link klangFunctionCall Function
hi def link klangString String
hi def link klangChar Character
hi def link klangHereString String
hi def link klangEscape SpecialChar
hi def link klangInvalidEscape Error

let b:current_syntax = 'klang'
