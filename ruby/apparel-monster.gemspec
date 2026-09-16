# frozen_string_literal: true

require_relative "lib/apparel_monster/version"

Gem::Specification.new do |spec|
  spec.name    = "apparel-monster"
  spec.version = ApparelMonster::VERSION
  spec.authors = ["Michael Lugassy"]

  spec.summary = "SDK and CLI for the Apparel Monster agent commerce API."
  spec.description = <<~TEXT
    Search the catalog, build a cart, place an order, mint an Apple Pay / Google Pay
    link, watch a price, or ask in natural language. No authentication: every call
    except order history is anonymous. Standard library only, no runtime dependencies.
    apparel.monster is a demo storefront — orders are placed against a test gateway
    and nothing is ever fulfilled.
  TEXT

  spec.homepage = "https://apparel.monster/developers"
  spec.license  = "MIT"
  spec.required_ruby_version = ">= 2.7.0"

  spec.metadata = {
    "homepage_uri" => "https://apparel.monster/developers",
    "source_code_uri" => "https://github.com/mluggy/apparel-monster-dev",
    "bug_tracker_uri" => "https://github.com/mluggy/apparel-monster-dev/issues",
    "documentation_uri" => "https://apparel.monster/developers",
    "rubygems_mfa_required" => "false"
  }

  spec.files = Dir["lib/**/*.rb", "exe/*", "LICENSE", "README.md"]
  spec.bindir = "exe"
  spec.executables = ["apparel-monster"]
  spec.require_paths = ["lib"]
end
