/**
 * @file OpenFGA authorization model DSL
 * @license Apache-2.0
 *
 * Mirrors the reference ANTLR grammar at
 * https://github.com/openfga/language (OpenFGALexer.g4 / OpenFGAParser.g4),
 * schema 1.1 and 1.2 (modular models).
 *
 * Deliberate deviations, all in the direction of leniency so that a
 * half-typed buffer still yields a usable tree:
 *   - newlines are not significant; the reference grammar separates
 *     declarations with NEWLINE, we rely on keywords instead
 *   - type definitions and conditions may interleave
 *   - `or` / `and` / `but not` chains are not checked for homogeneity
 * `fga model validate` remains the authority on what is actually legal.
 */

/// <reference types="tree-sitter-cli/dsl" />
// @ts-check

const PREC = {
  ternary: 1,
  or: 2,
  and: 3,
  comparison: 4,
  additive: 5,
  multiplicative: 6,
  unary: 7,
  postfix: 8,
};

const CEL_SCALAR_TYPES = [
  'bool',
  'string',
  'int',
  'uint',
  'double',
  'duration',
  'timestamp',
  'ipaddress',
];

/**
 * @param {string} sep
 * @param {RuleOrLiteral} rule
 */
function sepBy1(sep, rule) {
  return seq(rule, repeat(seq(sep, rule)));
}

module.exports = grammar({
  name: 'fga',

  word: ($) => $.identifier,

  extras: ($) => [/\s/, $.comment],

  supertypes: ($) => [$._relation_def, $._operand],

  rules: {
    source_file: ($) =>
      seq(
        optional(choice($.model_header, $.module_header)),
        repeat(choice($.type_definition, $.condition)),
      ),

    // ---------------------------------------------------------------- headers

    model_header: ($) =>
      seq('model', 'schema', field('version', $.schema_version)),

    schema_version: (_) => /\d+\.\d+/,

    module_header: ($) => seq('module', field('name', $.identifier)),

    // ------------------------------------------------------- type definitions

    type_definition: ($) =>
      seq(
        optional('extend'),
        'type',
        field('name', $._dsl_name),
        optional($.relations_block),
      ),

    relations_block: ($) => seq('relations', repeat1($.relation_definition)),

    relation_definition: ($) =>
      seq(
        'define',
        field('name', $._dsl_name),
        ':',
        field('value', $._relation_def),
      ),

    // ------------------------------------------------------ relation algebra
    //
    // A direct assignment `[...]` may only appear as the first operand, which
    // is why the right-hand operands come from a narrower set.

    _relation_def: ($) =>
      choice($.union, $.intersection, $.exclusion, $._operand),

    union: ($) => seq($._operand, repeat1(seq('or', $._operand_no_direct))),

    intersection: ($) =>
      seq($._operand, repeat1(seq('and', $._operand_no_direct))),

    exclusion: ($) => seq($._operand, $.but_not, $._operand_no_direct),

    but_not: (_) => token(seq('but', /[ \t\f]+/, 'not')),

    _operand: ($) =>
      choice(
        $.direct_assignment,
        $.computed_relation,
        $.tupleset_relation,
        $.grouping,
      ),

    // A parenthesised group may hold a direct assignment even here; the
    // reference grammar forbids it, `fga model validate` reports it.
    _operand_no_direct: ($) =>
      choice($.computed_relation, $.tupleset_relation, $.grouping),

    grouping: ($) => seq('(', $._relation_def, ')'),

    // `viewer`
    computed_relation: ($) => field('relation', $._dsl_name),

    // `viewer from parent`
    tupleset_relation: ($) =>
      seq(
        field('relation', $._dsl_name),
        'from',
        field('tupleset', $._dsl_name),
      ),

    // ------------------------------------------------------ type restrictions

    direct_assignment: ($) =>
      seq('[', optional(sepBy1(',', $.type_restriction)), ']'),

    type_restriction: ($) =>
      seq(
        field('type', $._dsl_name),
        optional(choice($.wildcard, $.relation_suffix)),
        optional($.condition_suffix),
      ),

    // `user:*`
    wildcard: (_) => seq(':', '*'),

    // `#assignee` -- the `#` outranks a comment so that `role#assignee`
    // lexes as a restriction rather than swallowing the rest of the line.
    relation_suffix: ($) =>
      seq(alias(token(prec(2, '#')), '#'), field('relation', $._dsl_name)),

    // `with non_expired` / `with $expression`
    condition_suffix: ($) =>
      seq(
        'with',
        field('condition', choice($.identifier, $.inline_expression)),
      ),

    inline_expression: (_) => '$expression',

    // ------------------------------------------------------------- conditions

    condition: ($) =>
      seq(
        'condition',
        field('name', $.identifier),
        $.parameter_list,
        field('body', $.condition_body),
      ),

    parameter_list: ($) => seq('(', optional(sepBy1(',', $.parameter)), ')'),

    parameter: ($) =>
      seq(field('name', $.identifier), ':', field('type', $.parameter_type)),

    parameter_type: ($) =>
      choice(
        $.scalar_type,
        seq(
          field('container', choice('map', 'list')),
          '<',
          field('element', $.scalar_type),
          '>',
        ),
      ),

    scalar_type: (_) => choice(...CEL_SCALAR_TYPES),

    condition_body: ($) => seq('{', $._expression, '}'),

    // ------------------------------------------------------- CEL expressions

    _expression: ($) =>
      choice(
        $.conditional,
        $.binary_expression,
        $.unary_expression,
        $._postfix_expression,
      ),

    conditional: ($) =>
      prec.right(
        PREC.ternary,
        seq(
          field('condition', $._expression),
          '?',
          field('consequence', $._expression),
          ':',
          field('alternative', $._expression),
        ),
      ),

    binary_expression: ($) => {
      const table = [
        [PREC.or, '||'],
        [PREC.and, '&&'],
        [PREC.comparison, choice('==', '!=', '<', '<=', '>', '>=', 'in')],
        [PREC.additive, choice('+', '-')],
        [PREC.multiplicative, choice('*', '/', '%')],
      ];

      return choice(
        ...table.map(([precedence, operator]) =>
          prec.left(
            Number(precedence),
            seq(
              field('left', $._expression),
              field('operator', operator),
              field('right', $._expression),
            ),
          ),
        ),
      );
    },

    unary_expression: ($) =>
      prec(
        PREC.unary,
        seq(field('operator', choice('!', '-')), field('operand', $._expression)),
      ),

    _postfix_expression: ($) =>
      choice(
        $.member_expression,
        $.index_expression,
        $.call_expression,
        $._primary_expression,
      ),

    member_expression: ($) =>
      prec(
        PREC.postfix,
        seq(field('object', $._postfix_expression), '.', field('property', $.identifier)),
      ),

    index_expression: ($) =>
      prec(
        PREC.postfix,
        seq(field('object', $._postfix_expression), '[', field('index', $._expression), ']'),
      ),

    call_expression: ($) =>
      prec(
        PREC.postfix,
        seq(
          field('function', $._postfix_expression),
          '(',
          optional(sepBy1(',', $._expression)),
          ')',
        ),
      ),

    _primary_expression: ($) =>
      choice(
        $.identifier,
        $.number,
        $.string,
        $.boolean,
        $.null,
        $.list,
        $.map,
        $.parenthesized_expression,
      ),

    parenthesized_expression: ($) => seq('(', $._expression, ')'),

    list: ($) => seq('[', optional(sepBy1(',', $._expression)), optional(','), ']'),

    map: ($) => seq('{', optional(sepBy1(',', $.map_entry)), optional(','), '}'),

    map_entry: ($) =>
      seq(field('key', $._expression), ':', field('value', $._expression)),

    // ----------------------------------------------------------------- tokens

    boolean: (_) => choice('true', 'false'),

    null: (_) => 'null',

    number: (_) =>
      token(
        choice(
          /0[xX][0-9a-fA-F]+[uU]?/,
          /\d+\.\d+([eE][+-]?\d+)?/,
          /\.\d+([eE][+-]?\d+)?/,
          /\d+[eE][+-]?\d+/,
          /\d+[uU]?/,
        ),
      ),

    string: (_) =>
      token(
        choice(
          seq('"""', /([^\\]|\\.)*?/, '"""'),
          seq("'''", /([^\\]|\\.)*?/, "'''"),
          seq('"', repeat(choice(/[^\\"\n\r]/, /\\./)), '"'),
          seq("'", repeat(choice(/[^\\'\n\r]/, /\\./)), "'"),
        ),
      ),

    // CEL, condition names and parameter names use the plain identifier;
    // `a.b` there is member access, not one name. Type and relation names use
    // the extended form, which does admit `.`, `/` and `-` inside a single
    // name. Both surface as `identifier` in the tree.
    identifier: (_) => /[A-Za-z_][A-Za-z0-9_-]*/,

    _dsl_name: ($) => alias($._extended_identifier, $.identifier),

    _extended_identifier: (_) =>
      token(/[A-Za-z_][A-Za-z0-9_]*([/.-]?[A-Za-z0-9_]+)*/),

    // `#` to end of line in the DSL, `//` to end of line inside a CEL body.
    comment: (_) => token(choice(seq('#', /[^\n]*/), seq('//', /[^\n]*/))),
  },
});
