# frozen_string_literal: true

# Smoke tests against the live API.
#
# Live, not mocked. A mocked client tests the mock; these assert the contract
# published at /api/v1/openapi.yaml, so a nightly red build means the store
# moved — which is what an SDK maintainer wants to hear first.
#
# Nothing here completes an order. The cart flow stops before checkout: this is
# a demo store, but a trail of CI orders would make its own reporting useless.

require "minitest/autorun"
require_relative "../lib/apparel_monster"

class SearchTest < Minitest::Test
  def setup
    @store = ApparelMonster::Client.new
  end

  def test_search_returns_products_and_mints_a_session
    result = @store.search("denim shirt", limit: 3)

    refute_nil result.dig("session", "id"), "no session id"
    refute_empty result["products"], "no products for a known-good query"

    product = result["products"].first
    refute_nil product["title"]
    assert product["url"].start_with?("https://apparel.monster/")
    refute_empty product["variants"]
    assert_kind_of Numeric, product["variants"].first["price"]

    # Carried from here on without the caller doing anything.
    assert_equal result.dig("session", "id"), @store.session_id
  end

  def test_identifies_itself
    headers = @store.headers
    assert headers["User-Agent"].start_with?("apparel-monster-ruby/")
    assert_equal "ruby-sdk/#{ApparelMonster::VERSION}", headers["X-Agent-Client"]
  end

  def test_cli_identifies_itself_differently
    headers = ApparelMonster::Client.new(cli: true).headers
    assert_includes headers["User-Agent"], "apparel-monster-ruby-cli/"
    assert_equal "ruby-cli/#{ApparelMonster::VERSION}", headers["X-Agent-Client"]
  end

  def test_product_accepts_a_variant_id
    found = @store.search("denim shirt", limit: 1)
    variant_id = found["products"].first["variants"].first["id"]

    refute_empty @store.product(variant_id)["products"]
  end
end

class CartTest < Minitest::Test
  def test_add_then_price_shipping
    store = ApparelMonster::Client.new
    found = store.search("denim shirt", limit: 1)
    variants = found["products"].first["variants"]
    variant_id = (variants.find { |v| v["available"] } || variants.first)["id"]

    added = store.add_to_cart(variant_id, 1)
    assert_operator added.dig("totals", "total"), :>, 0

    billed = store.set_billing(
      "email" => "sdk-ci@example.com",
      "first_name" => "CI",
      "last_name" => "Runner",
      "line_one" => "1 Market St",
      "city" => "San Francisco",
      "region_id" => "CA",
      "postal_code" => "94105",
      "country_id" => "US"
    )
    refute_empty billed["shipping_options"], "no shipping options after an address"
  end

  def test_missing_field_is_reported_as_a_missing_field
    store = ApparelMonster::Client.new
    found = store.search("denim shirt", limit: 1)
    store.add_to_cart(found["products"].first["variants"].first["id"], 1)

    error = assert_raises(ApparelMonster::Error) { store.set_billing("line_one" => "1 Market St") }

    assert_equal 422, error.status
    assert_equal "missing_fields", error.code

    fields = error.missing.map { |m| m["field"] }
    assert_includes fields, "city"
    assert_includes fields, "postal_code"

    # The whole point: the message a model reads names them too.
    assert_includes error.message, "city"
    assert_includes error.message, "postal_code"
  end
end

class AskTest < Minitest::Test
  def test_ask_returns_schema_org_products
    answer = ApparelMonster::Client.new.ask("denim shirt", limit: 2)

    assert_equal "result_list", answer.dig("_meta", "response_type")
    assert_equal "https://apparel.monster", answer.dig("_meta", "site")
    return if answer["results"].empty?

    assert_equal "Product", answer["results"].first.dig("schema_object", "@type")
  end

  def test_ask_stream_starts_and_completes
    events = []
    ApparelMonster::Client.new.ask_stream("coat", limit: 2) { |event, _data| events << event }

    assert_equal "start", events.first
    assert_equal "complete", events.last
  end
end

class BatchTest < Minitest::Test
  def test_batch_runs_several_operations
    result = ApparelMonster::Client.new.batch(
      [
        { "id" => "a", "path" => "/api/v1/search", "body" => { "query" => "denim", "limit" => 1 } },
        { "id" => "b", "path" => "/api/v1/search", "body" => { "query" => "coat", "limit" => 1 } }
      ]
    )

    assert_equal 2, result["requested"]
    assert(result["results"].all? { |r| r["status"] == 200 })
  end

  def test_batch_refuses_more_than_the_maximum_before_sending
    operations = Array.new(11) { { "path" => "/api/v1/search", "body" => {} } }

    assert_raises(ArgumentError) { ApparelMonster::Client.new.batch(operations) }
  end
end
