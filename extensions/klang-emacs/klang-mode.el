;;; klang-mode.el --- Major mode for Klang files -*- lexical-binding: t; -*-

;; Author: kLang contributors
;; Version: 0.1.0
;; Keywords: languages

;;; Commentary:

;; Syntax highlighting, indentation, comments, and simple skeletons for
;; the kLang programming language.

;;; Code:

(require 'rx)
(require 'skeleton)

(defgroup klang nil
  "Major mode for editing Klang source files."
  :group 'languages)

(defcustom klang-indent-offset 4
  "Indentation offset for `klang-mode'."
  :type 'integer
  :group 'klang)

(defconst klang--keywords
  '("if" "then" "else" "unless" "for" "while" "do" "do_while" "end"
    "return" "break" "continue" "try" "catch" "throw" "case" "partial"
    "import" "namespace" "region" "alias" "call" "function_group"
    "await" "defer" "private" "inline" "lazy" "async" "inner"))

(defconst klang--operators
  '("and" "or" "xor" "not" "is" "as" "in" "move" "copy" "clone"))

(defconst klang--storage
  '("global" "local" "let" "var" "val" "const" "mut" "export"))

(defconst klang--types
  '("Int" "UInt" "String" "Float" "Bool" "Char" "Map" "List" "Table"
    "Option" "Result" "Complex" "SIMD" "Function" "Awaitable" "Iterator"
    "Coroutine" "Thread" "Atomic" "Any" "T" "Args" "Program" "BuildSystem"
    "WorkSpace" "JSModule" "JSCall" "Box" "Ref" "RefMut" "RefCell"
    "HeapAllocator" "RegionAllocator" "BumpAllocator" "ArenaAllocator"))

(defconst klang--constants
  '("True" "False" "None" "Some" "Ok" "Err" "Result" "DEFAULT"))

(defconst klang--builtins
  '("print" "input" "len" "range" "iter" "next" "coroutine" "resume"
    "spawn" "join" "thread_status" "Atomic" "atomic_load" "atomic_store" "atomic_add" "Program" "BuildSystem"
    "WorkSpace" "workspace_backend" "workspace_files" "workspace_manifest"
    "debug" "debug_type" "debug_stack" "breakpoint"
    "js_import" "js_source" "js_exports" "js_call"
    "get_default_procces_allocator" "free_all_allocator"))

(defconst klang-font-lock-keywords
  `((,(regexp-opt klang--keywords 'symbols) . font-lock-keyword-face)
    (,(regexp-opt klang--operators 'symbols) . font-lock-builtin-face)
    (,(regexp-opt klang--storage 'symbols) . font-lock-variable-name-face)
    (,(regexp-opt klang--types 'symbols) . font-lock-type-face)
    (,(regexp-opt klang--constants 'symbols) . font-lock-constant-face)
    (,(regexp-opt klang--builtins 'symbols) . font-lock-builtin-face)
    ("\\[\\(new\\|delete\\|side_effects\\)\\]" . font-lock-preprocessor-face)
    ("#\\(extend\\|region\\|sizeof\\|call_site\\|set_entry_point_to_here\\)\\_>" . font-lock-preprocessor-face)
    ("@deprecated\\(?:([^)]*)\\)?" . font-lock-preprocessor-face)
    ("\\_<alias\\_>\\s-+\\_<function\\_>\\s-+\\([A-Za-z_][A-Za-z0-9_]*\\)" 1 font-lock-type-face)
    ("\\_<function\\_>\\s-+\\([A-Za-z_][A-Za-z0-9_]*\\)" 1 font-lock-function-name-face)
    ("\\_<namespace\\_>\\s-+\\([A-Za-z_][A-Za-z0-9_]*\\)" 1 font-lock-constant-face)
    ("\\_<trait\\_>\\s-+\\([A-Za-z_][A-Za-z0-9_]*\\)" 1 font-lock-type-face)
    ("\\_<impl\\_>\\s-+\\([A-Za-z_][A-Za-z0-9_]*\\)" 1 font-lock-type-face)
    ("\\_<region\\_>\\s-+\\([A-Za-z_][A-Za-z0-9_]*\\)" 1 font-lock-constant-face)
    ("\\_<\\([A-Za-z_][A-Za-z0-9_]*\\)\\_>\\s-*(" 1 font-lock-function-name-face)))

(defvar klang-mode-syntax-table
  (let ((table (make-syntax-table)))
    (modify-syntax-entry ?_ "w" table)
    (modify-syntax-entry ?- ". 12" table)
    (modify-syntax-entry ?\n ">" table)
    (modify-syntax-entry ?\" "\"" table)
    (modify-syntax-entry ?' "\"" table)
    table)
  "Syntax table for `klang-mode'.")

(defun klang--line-opens-block-p ()
  "Return non-nil when the current line opens a Klang block."
  (save-excursion
    (back-to-indentation)
    (looking-at-p ".*\\({\\|\\[\\|(\\|\\_<do\\_>\\|\\_<then\\_>\\|\\_<try\\_>\\|\\_<catch\\_>\\)\\s-*\\(?:--.*\\)?$")))

(defun klang--line-closes-block-p ()
  "Return non-nil when the current line closes a Klang block."
  (save-excursion
    (back-to-indentation)
    (looking-at-p "\\(}\\|]\\|)\\|\\_<end\\_>\\|\\_<else\\_>\\|\\_<catch\\_>\\|\\_<case\\_>\\)")))

(defun klang-calculate-indentation ()
  "Compute indentation for the current line."
  (save-excursion
    (beginning-of-line)
    (if (bobp)
        0
      (let ((indent 0))
        (forward-line -1)
        (while (and (not (bobp)) (looking-at-p "^\\s-*$"))
          (forward-line -1))
        (setq indent (current-indentation))
        (when (klang--line-opens-block-p)
          (setq indent (+ indent klang-indent-offset)))
        (forward-line 1)
        (when (klang--line-closes-block-p)
          (setq indent (- indent klang-indent-offset)))
        (max indent 0)))))

(defun klang-indent-line ()
  "Indent current line as Klang source."
  (interactive)
  (let ((indent (klang-calculate-indentation))
        (offset (- (current-column) (current-indentation))))
    (indent-line-to indent)
    (when (> offset 0)
      (move-to-column (+ indent offset)))))

;;;###autoload
(define-derived-mode klang-mode prog-mode "Klang"
  "Major mode for Klang source files."
  :syntax-table klang-mode-syntax-table
  (setq-local font-lock-defaults '(klang-font-lock-keywords))
  (setq-local indent-line-function #'klang-indent-line)
  (setq-local comment-start "-- ")
  (setq-local comment-end "")
  (setq-local comment-start-skip "--+\\s-*"))

;;;###autoload
(add-to-list 'auto-mode-alist '("\\.klang\\'" . klang-mode))

(define-skeleton klang-insert-function
  "Insert a Klang function template."
  "Function name: "
  "function " str "(value : Int) : Int {" \n
  "    return value;" \n
  "}" \n)

(define-skeleton klang-insert-alias-function
  "Insert a Klang alias function template."
  "Alias name: "
  "alias function " str "[T: Any](data: T) : type {" \n
  "    [new] {" \n
  "        " _ \n
  "    }" \n \n
  "    #extend {" \n
  "        function get_value() -> T {" \n
  "            return this.data;" \n
  "        }" \n
  "    }" \n
  "}" \n)

(define-skeleton klang-insert-main
  "Insert a Klang Main function template."
  nil
  "function Main() : Int {" \n
  "    return 0;" \n
  "}" \n)

(provide 'klang-mode)

;;; klang-mode.el ends here
