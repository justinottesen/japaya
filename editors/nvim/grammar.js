module.exports = grammar({
  name: 'japaya',

  rules: {
    program: $ => repeat(
      choice(
        $.python_block,
        $.python_inline,
        $.content,
      )
    ),

    // Python block with triple backticks
    python_block: $ => seq(
      '```',
      optional($.python_block_content),
      '```',
    ),

    python_block_content: $ => repeat1(choice(
      /[^`]+/,
      /`[^`]/,
      /``[^`]/,
    )),

    // Python inline with single backtick
    python_inline: $ => seq(
      '`',
      optional($.python_inline_content),
      '`',
    ),

    python_inline_content: $ => /[^`]+/,

    // Everything else (Java content)
    content: $ => /[^`]+/,
  }
});
