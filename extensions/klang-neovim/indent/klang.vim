if exists('b:did_indent')
  finish
endif
let b:did_indent = 1

setlocal autoindent
setlocal indentexpr=KlangIndent(v:lnum)
setlocal indentkeys=o,O,0},0],0),0=end,0=else,0=catch,0=case

if exists('*KlangIndent')
  finish
endif

function! KlangIndent(lnum) abort
  let l:prevnum = prevnonblank(a:lnum - 1)
  if l:prevnum == 0
    return 0
  endif

  let l:prev = getline(l:prevnum)
  let l:current = getline(a:lnum)
  let l:indent = indent(l:prevnum)

  if l:prev =~# '\v(\{|\[|\(|\bdo|\bthen|\btry|\bcatch)\s*(--.*)?$'
    let l:indent += shiftwidth()
  endif

  if l:prev =~# '^\s*\[\(new\|delete\|side_effects\)\]\s*{'
    let l:indent += shiftwidth()
  endif

  if l:prev =~# '^\s*case\>'
    let l:indent += shiftwidth()
  endif

  if l:current =~# '^\s*\(}\|]\|)\|end\>\|else\>\|catch\>\|case\>\)'
    let l:indent -= shiftwidth()
  endif

  return max([l:indent, 0])
endfunction

let b:undo_indent = 'setlocal autoindent< indentexpr< indentkeys<'
