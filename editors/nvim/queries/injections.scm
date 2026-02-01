; queries/injections.scm
; Language injection for Japaya

; Inject Python into triple backtick blocks
((python_block_content) @injection.content
  (#set! injection.language "python"))

; Inject Python into single backtick inline
((python_inline_content) @injection.content
  (#set! injection.language "python"))

; Inject Java into regular content
((content) @injection.content
  (#set! injection.language "java"))
