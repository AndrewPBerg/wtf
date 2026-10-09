-- Explicit registration: filenames have no special meaning.
return {
  hooks = {
	init = require("initialize"),
    guard = require("guard"),
    prepare = require("prepare"),
    start = require("start"),
    test = require("test"),
    stop = require("stop"),
  },
  overrides = {
    new = require("new"),
    remove = require("remove"),
  },
}
