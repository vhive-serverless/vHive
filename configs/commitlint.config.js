const Configuration = {
    /*
     * Any rules defined here will override rules from @commitlint/config-conventional
     */
    rules: {
        "header-max-length": [0, "always", 72],
        "header-min-length": [0, "always", 10],
        'signed-off-by': [1, 'always', 'Signed-off-by:'],
    },
};

module.exports = Configuration;