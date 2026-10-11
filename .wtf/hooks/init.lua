-- Explicit registration: filenames have no special meaning.
return {
  hooks = {
    guard = require("guard"),
    prepare = require("prepare"),
    start = require("start"),
    test = require("test"),
    stop = require("stop"),
  },
  overrides = {
    init = require("initialize"),
    remove = require("remove"),
  },
}
