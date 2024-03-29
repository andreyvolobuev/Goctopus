package main

// TODO: REFACTOR ALL OF THESE CONSTANTS. SOME MIGHT BE JOINED
const (
	ERR_TEMPLATE             = "%s"
	AUTH_FAILED              = "Authentication failed: %s"
	POST_NEW_MSG             = "[POST] New message!"
	NO_CREDS_FOR_POST        = "Credentials for POST-request not provided!"
	BAD_CREDS_FOR_POST       = "POST-request with bad credentials"
	AUTH_INVALID_CREDS       = "invalid credentials"
	NEW_MSG_CREATED          = "New message: %s"
	GET_ALL_MSGS             = "[GET] list of all available keys in the storage"
	GET_MSGS_FROM_KEY        = "[GET] list all messages from key: %s"
	ID_BUT_NO_KEY_ERR        = "can not handle delete request where message id %s is provided, but key is not."
	ALL_DELETED              = "deleted all messages from all keys in storage"
	ALL_DELETED_FROM_KEY     = "deleted all messages from key: %s"
	DELETED_MSG              = "deleted message with id: %s from key: %s"
	INVALID_UUID             = "provided id: %s is not a valid uuid"
	METHOD_NOT_ALLOWED       = "[%s] METHOD IS NOT ALLOWED!"
	START_APP                = "starting Goctopus websocket app..."
	WS_WORKERS_NOT_FOUND     = "Can not parse WS_WORKERS! The value has to be an integer."
	START_SENDING            = "Start sending messages for %s"
	NO_CONNS                 = "No active connections for %s. Return"
	ERR_GET_MSGS             = "Could not get messages for %s. Return"
	NO_MSGS                  = "No messages to send for %s. Return"
	TRY_SENDING_MSG          = "Try sending message id: %s, value: %s, to %s"
	MSG_EXPIRED              = "Message id: %s is expired and will be discarted"
	MARSHAL_ERR              = "%s. Will discard this message from queue for %s"
	CONN_ERR                 = "%s. Will remove a conn from %s"
	ALL_SENT                 = "All messages for %s have been sent, the queue is empty"
	NOT_ALL_SENT             = "There are %d unsent messages tha will remain in the queue for %s"
	ALL_CONNS_CLOSED         = "All connections for %s have been closed"
	NOT_ALL_CONNS_CLOSED     = "There are %d active connections remain for %s"
	COULD_NOT_CONVERT_TO_ERR = "could not convert message id to %s"
	WRONG_ID_CONFIRM         = "received confirmation for wrong id (expected for %s, got for %s)"
	SAVED_NEW_CONN           = "Saved new connection for %s"
	STORAGE_INITIALIZED      = "Storage %s initialized"
	AUTHORIZER_INITIALIZED   = "Authorizer %s will be used"
	LIST_KEYS_ERR            = "could not get list of keys for %s"
	INVALID_KEY              = "invalid message key"
	INVALID_VALUE            = "invalid message value"
	MESSAGE                  = "message"
	MULTIPLE                 = "s"
	ALL                      = "all "
	IN_STORAGE               = " in storage"
	WITH_ID                  = " with id: %s"
	FROM_KEY                 = " from key: %s"
	DELETE_METHOD            = "[DELETE] "

	// literals
	BYTES     = "bytes"
	UUID      = "uuid"
	DEFAULT   = "default"
	MEMORY    = "memory"
	DUMMY     = "dummy"
	PROXY     = "proxy"
	EMPTY_STR = ""

	// environments variable names
	WS_WORKERS    = "WS_WORKERS"
	WS_VERBOSE    = "WS_VERBOSE"
	WS_HOST       = "WS_HOST"
	WS_PORT       = "WS_PORT"
	WS_MSG_EXPIRE = "WS_MSG_EXPIRE"
	WS_AUTHORIZER = "WS_AUTHORIZER"
	WS_STORAGE    = "WS_STORAGE"
	WS_LOGIN      = "WS_LOGIN"
	WS_PASSWORD   = "WS_PASSWORD"
	WS_AUTH_URL   = "WS_AUTH_URL"

	// query parameters
	KEY = "key"
	ID  = "id"

	// routes
	ROOT = "/"
	WS   = "/ws"
	WS_  = "/ws/"

	// headers
	CONTENT_TYPE     = "Content-Type"
	APPLICATION_JSON = "application/json"
)
