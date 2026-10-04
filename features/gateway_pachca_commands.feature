Feature: Pachca chat commands
  The bot takes a few commands of its own - /clear starts a new session,
  /model shows the configured models as buttons and a click switches the
  conversation's model - and hands the settings commands (/plan, /think, ...)
  to the session like a message. In a group chat a command needs no mention.

  Background:
    Given a fake Pachca workspace whose bot is "coddy_bot"
    And a pachca gateway over a scripted agent pointed at it
    And the bot is started

  Scenario: /clear starts a new session
    Given the person "anna" wrote "hello" in a direct chat and got an answer
    When the person "anna" writes "/clear" in a direct chat
    Then the direct chat with "anna" shows a bot message "New session started."
    And the next message of "anna" runs in a new session

  Scenario: /model offers the models and a click switches the session
    When the person "anna" writes "/model" in a direct chat
    Then the direct chat with "anna" shows a model menu with a button for "rpa/qwen3.6-35b-a3b"
    When "anna" clicks the button for "rpa/qwen3.6-35b-a3b"
    Then the session model of "anna" is "rpa/qwen3.6-35b-a3b"
    And the model menu marks "rpa/qwen3.6-35b-a3b" as current

  Scenario: A command in a group needs no mention
    When the person "boris" writes "/help" in the group "dev"
    Then the group "dev" shows a bot message containing "/clear"
