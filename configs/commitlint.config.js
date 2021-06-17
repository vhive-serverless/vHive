const Configuration = {

    // The configuration array contains:

    // Level [0..2]: 0 disables the rule. For 1 it will be considered a warning for 2 an error.
    // Applicable [always|never]: never inverts the rule.
    // Value: value to use for this rule.
    // Source:
    // https://commitlint.js.org/#/reference-rules

    /*
     * Any rules defined here will override rules from @commitlint/config-conventional
     */

    rules: {
        "header-max-length": [2, "always", 72],
        "header-min-length": [2, "always", 10],
        'signed-off-by': [2, 'always', 'Signed-off-by:'],
    },
};

module.exports = Configuration;
