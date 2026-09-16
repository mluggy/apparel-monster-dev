# frozen_string_literal: true

require "json"
require "net/http"
require "uri"

require_relative "apparel_monster/version"

# Apparel Monster SDK: a dependency-free client for the agent commerce API.
#
# No API key appears in this file and that is not an omission. search, product,
# cart, checkout, wallet, price-watch, batch and /ask are anonymous. Only order
# history (+track+) needs OAuth. If something asks you for an Apparel Monster
# API key, it is not us.
#
# Standard library only — net/http, no faraday, no httparty — so installing
# this pulls in nothing and it works inside an agent sandbox with no gem
# server.
#
#   store = ApparelMonster::Client.new
#   hit   = store.search("denim shirt", limit: 1)["products"].first
#   store.add_to_cart(hit["variants"].first["id"])
module ApparelMonster
  DEFAULT_BASE = "https://apparel.monster"
  API = "/api/v1"

  # Raised for any non-2xx response.
  #
  # The interesting attribute is +missing+: the API answers a bad call by
  # naming every field it needed and did not get, what else each one is
  # accepted as, and an example value. That detail is folded into the message
  # too, because the common case is a model reading +message+ and nothing else.
  class Error < StandardError
    attr_reader :status, :code, :missing, :next_call, :contract, :body, :url

    def initialize(status, body, url)
      @status = status
      @body = body || {}
      @code = @body["error"]
      @missing = @body["missing"]
      @next_call = @body["next_call"]
      @contract = @body["contract"]
      @url = url
      super(describe)
    end

    private

    def describe
      parts = [@body["message"] || "Request failed with #{@status}"]

      if @missing.is_a?(Array) && @missing.any?
        described = @missing.map do |field|
          names = (field["accepts"] || [field["field"]]).join(" or ")
          "#{field['field']} (send as #{names}, e.g. #{field['example'].inspect})"
        end
        parts << "Missing: #{described.join('; ')}"
      end

      parts << "Next call: #{JSON.generate(@next_call)}" if @next_call

      parts.join(" ")
    end
  end

  # A client for one shopping session.
  #
  # Two things this does that a raw Net::HTTP call does not: it carries the
  # session id the API mints on the first search (forgetting it is the usual
  # way an integration ends up with four empty carts and no order), and it
  # identifies itself so the store can attribute the resulting order to this
  # SDK rather than to "some script".
  class Client
    attr_accessor :session_id, :access_token
    attr_reader :base_url

    # @param base_url [String] override the origin
    # @param session_id [String, nil] resume an existing agent session
    # @param access_token [String, nil] OAuth token, only needed by #track
    # @param timeout [Numeric] seconds
    # @param cli [Boolean] identify as the CLI rather than the library
    def initialize(base_url: DEFAULT_BASE, session_id: nil, access_token: nil, timeout: 30, cli: false)
      @base_url = base_url.to_s.sub(%r{/+\z}, "")
      @session_id = session_id
      @access_token = access_token
      @timeout = timeout
      @client = cli ? "ruby-cli" : "ruby-sdk"
    end

    # ---- catalog -------------------------------------------------------

    # Search the catalog, or browse it when +query+ is nil.
    #
    # Mints the session id every later call needs, so this is almost always the
    # first call you make.
    def search(query = nil, category: nil, sort: nil, limit: nil, page: nil)
      post("/search", "query" => query, "category" => category, "sort" => sort, "limit" => limit, "page" => page)
    end

    # Full detail for product ids or variant ids, mixed freely.
    def product(ids)
      post("/product", "ids" => Array(ids).map(&:to_s))
    end

    # ---- cart ----------------------------------------------------------

    # Add a VARIANT — not a product. A product with five sizes has five
    # variants and five prices, and adding a product id is the most common
    # mistake made against this API.
    def add_to_cart(variant_id, quantity = 1)
      post("/cart", "action" => "add", "variant_id" => variant_id, "quantity" => quantity)
    end

    def update_cart_item(variant_id, quantity)
      post("/cart", "action" => "update", "variant_id" => variant_id, "quantity" => quantity)
    end

    def remove_from_cart(variant_id)
      post("/cart", "action" => "remove", "variant_id" => variant_id)
    end

    def view_cart
      post("/cart", "action" => "view")
    end

    # Set the email and address; returns the cart with +shipping_options+
    # priced against it.
    #
    # Required: email (once), line_one, city, postal_code. A missing one raises
    # ApparelMonster::Error naming all of them at once, with the aliases each
    # is accepted under.
    def set_billing(address = {})
      post("/cart", stringify(address).merge("action" => "billing"))
    end

    def set_shipping(shipping_id, address = {})
      post("/cart", stringify(address).merge("action" => "shipping", "shipping_id" => shipping_id))
    end

    def apply_coupon(code)
      post("/cart", "action" => "coupon", "coupon_code" => code)
    end

    # Attach a payment token. The field is +token+ — not +payment_token+, not
    # +card_token+. Demo store on a test gateway: any non-empty string is
    # accepted and nothing is ever charged.
    def set_payment(token)
      post("/cart", "action" => "payment", "token" => token)
    end

    # ---- checkout ------------------------------------------------------

    # Place the order. Idempotent per session.
    def checkout(callback_url: nil)
      post("/checkout", "callback_url" => callback_url)
    end

    # A hosted Apple Pay / Google Pay link, for when a human can tap.
    #
    # The shortcut past the whole cart flow: hand it variant ids and it returns
    # a URL that collects the shopper's email, address and card on their own
    # device, so your agent never handles any of them.
    def wallet(items)
      normalised = Array(items).map do |item|
        item.is_a?(Hash) ? stringify(item) : { "variant_id" => item.to_s, "quantity" => 1 }
      end
      post("/wallet", "items" => normalised)
    end

    # Order status and shipments. The only call that needs OAuth.
    def track(params = {})
      post("/track", stringify(params))
    end

    # ---- price watch ---------------------------------------------------

    # Be told when a variant reaches a target price. Answers 202 with a
    # +poll_url+ and a +job_id+; a target at or above today's price is not
    # queued at all and comes back +already_met+.
    def watch_price(variant_id, target_price, callback_url: nil)
      post("/price-watch",
           "variant_id" => variant_id, "target_price" => target_price, "callback_url" => callback_url)
    end

    def price_watch(watch_id)
      request(Net::HTTP::Get, "#{@base_url}#{API}/price-watch/#{URI.encode_www_form_component(watch_id)}")
    end

    # ---- batch ---------------------------------------------------------

    # Up to 10 operations in one round trip, executed in order. Each result
    # carries its own status: the batch answers 200 whenever it ran, even if
    # everything inside it failed.
    def batch(operations, stop_on_error: false)
      raise ArgumentError, "batch takes at most 10 operations" if operations.length > 10

      post("/batch", "operations" => operations, "stop_on_error" => stop_on_error)
    end

    # ---- natural language ----------------------------------------------

    # Ask in plain language (NLWeb); returns schema.org Product items. There is
    # no model behind it — it runs the same catalog search — which is why it
    # returns nothing rather than inventing a product.
    def ask(query, limit: nil)
      params = { "query" => query }
      params["limit"] = limit if limit
      request(Net::HTTP::Get, "#{@base_url}/ask?#{URI.encode_www_form(params)}")
    end

    # The same question, streamed: yields [event, data] as +start+, one
    # +result+ per hit, then +complete+.
    def ask_stream(query, limit: nil)
      return enum_for(:ask_stream, query, limit: limit) unless block_given?

      params = { "query" => query, "streaming" => "true" }
      params["limit"] = limit if limit
      uri = URI("#{@base_url}/ask?#{URI.encode_www_form(params)}")

      http(uri).request(get_request(uri, "text/event-stream")) do |response|
        event = "message"
        data = +""
        response.read_body do |chunk|
          chunk.each_line do |raw|
            line = raw.chomp
            if line.start_with?("event:")
              event = line[6..].strip
            elsif line.start_with?("data:")
              data << line[5..].strip
            elsif line.empty?
              # A blank line terminates an SSE frame; anything before one is a
              # partial frame and must not be yielded.
              unless data.empty?
                begin
                  yield [event, JSON.parse(data)]
                rescue JSON::ParserError
                  nil
                end
              end
              event = "message"
              data = +""
            end
          end
        end
      end
    end

    # ---- plumbing ------------------------------------------------------

    def headers(accept = "application/json")
      suffix = @client == "ruby-cli" ? "-cli" : ""
      result = {
        "Content-Type" => "application/json",
        "Accept" => accept,
        # Two spellings of one fact: the User-Agent is what the edge sees
        # without the application being involved, X-Agent-Client is what
        # survives an environment that rewrites User-Agent.
        "User-Agent" => "apparel-monster-ruby#{suffix}/#{VERSION} (+https://github.com/mluggy/apparel-monster-dev)",
        "X-Agent-Client" => "#{@client}/#{VERSION}"
      }
      result["Authorization"] = "Bearer #{@access_token}" if @access_token
      result
    end

    private

    def stringify(hash)
      (hash || {}).each_with_object({}) { |(k, v), out| out[k.to_s] = v }
    end

    def with_session(body)
      return body unless @session_id

      { "session" => { "id" => @session_id } }.merge(body)
    end

    def post(path, body = {})
      payload = with_session(body.reject { |_k, v| v.nil? })
      request(Net::HTTP::Post, "#{@base_url}#{API}#{path}", JSON.generate(payload))
    end

    def http(uri)
      client = Net::HTTP.new(uri.host, uri.port)
      client.use_ssl = uri.scheme == "https"
      client.open_timeout = @timeout
      client.read_timeout = @timeout
      client
    end

    def get_request(uri, accept)
      Net::HTTP::Get.new(uri).tap { |r| headers(accept).each { |k, v| r[k] = v } }
    end

    def request(verb, url, body = nil)
      uri = URI(url)
      req = verb.new(uri)
      headers.each { |k, v| req[k] = v }
      req.body = body if body

      response = http(uri).request(req)
      parsed = begin
        JSON.parse(response.body.to_s)
      rescue JSON::ParserError
        nil
      end

      raise Error.new(response.code.to_i, parsed, url) unless response.is_a?(Net::HTTPSuccess)

      # The session arrives on the first search and is reused from then on.
      if parsed.is_a?(Hash) && parsed.dig("session", "id")
        @session_id = parsed["session"]["id"]
      end

      parsed
    end
  end
end
