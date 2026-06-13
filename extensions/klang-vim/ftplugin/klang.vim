if exists('b:did_ftplugin')
  finish
endif
let b:did_ftplugin = 1

setlocal commentstring=--\ %s
setlocal comments=:--
setlocal formatoptions-=t
setlocal suffixesadd=.klang

if !exists('b:undo_ftplugin')
  let b:undo_ftplugin = ''
endif
let b:undo_ftplugin .= '|setlocal commentstring< comments< formatoptions< suffixesadd<'

function! s:KlangInsert(lines) abort
  call append(line('.') - 1, a:lines)
  normal! j
  startinsert
endfunction

inoremap <buffer> <leader>kf <Esc>:call <SID>KlangInsert(['function Name(value : Int) : Int {', '    return value;', '}'])<CR>
inoremap <buffer> <leader>ka <Esc>:call <SID>KlangInsert(['alias function Name[T: Any](data: T) : type {', '    [new] {', '        ', '    }', '', '    #extend {', '        function get_value() -> T {', '            return this.data;', '        }', '    }', '}'])<CR>
inoremap <buffer> <leader>ki <Esc>:call <SID>KlangInsert(['if condition {', '    ', '} else {', '    ', '}'])<CR>
inoremap <buffer> <leader>km <Esc>:call <SID>KlangInsert(['function Main() : Int {', '    return 0;', '}'])<CR>

