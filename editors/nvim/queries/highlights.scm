; queries/highlights.scm
; Highlight rules for Japaya

; Highlight the backtick delimiters
(python_block
  ["```"] @punctuation.bracket)

(python_inline
  ["`"] @punctuation.bracket)
