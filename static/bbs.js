var bbsModule = angular.module('bbsview', []).
  config(['$routeProvider', function($routeProvider) {
  $routeProvider.
      when('/servers', {templateUrl: '/static/server-list.html',   controller: ServerCtrl}).
      when('/login', {templateUrl: '/static/login.html', controller: LoginCtrl}).
      when('/boards', {templateUrl: '/static/board-list.html', controller: BoardCtrl}).
      when('/threads/:query', {templateUrl: '/static/thread-list.html', controller: ThreadListCtrl}).
      when('/thread/:id', {templateUrl: '/static/thread.html', controller: ThreadCtrl}).
      when('/thread/:id/range/:range', {templateUrl: '/static/thread.html', controller: ThreadCtrl}).
      otherwise({redirectTo: '/servers'});
}]);


var client = null;
bbsModule.service('bbs', function($http, $location) {
	if (client == null) {
		client = new BBSClient($http, $location);
	}
	return client;
})

function BBSClient($http, $location) {
	this.addr = null;
	this.session = null;
	this.user = null;
	this.server = null;

	this.connected = function() {
		return !!this.addr;
	}

	this.send = function(cmd, successCallback, errorCallback) {
		if (this.connected()) {
			if (this.session) {
				cmd.session = this.session;
			}
			$http.post(this.addr, cmd).
				success(successCallback).
				error(function(data) {
					//this is a kind of ghetto interceptor
					if (data.cmd == "error" && data.wrt == "session") {
						this.session = null;
						this.user = null;
						delete localStorage["sesh for " + this.addr];

						$location.path("#/login");
					} else if (errorCallback != null) {
						errorCallback(data);
					}
				});
		} else {
			console.log("Error: Not connected!!");
			console.log(this);
			$location.path("#/servers");
		}
	}

	this.parseRange = function(r) {
		var split = r.split("-", 2);
		if (split.length != 2) {
			return { start: 0, end: 0 }
		}
		return { start: parseInt(split[0], 10), end: parseInt(split[0], 10) }
	}

	this.range2String = function(r) {
		return r.start + "-" + r.end;
	}

	this.nextRange = function(r) {
		var diff = r.end - r.start + 1;
		return { start: r.start + diff, end: r.end + diff };
	}

	//try to restore the last sesion
	if (localStorage["last sesh"]) {
		this.addr = localStorage["last sesh"];
		this.session = localStorage["sesh for " + this.addr];
		this.server = JSON.parse(localStorage["data for " + this.addr])
		this.home = this.server.home;
	}
}

function ThreadListCtrl($scope, bbs, $location, $routeParams) {
	$scope.threads = {};
	$scope.query = $routeParams.query;

	$scope._list = function(data) {
		$scope.threads = data.threads;
	}

	$scope._error = function(data) {
		$scope.errorMsg = data.error;
	}

	var threadListCmd = {
		cmd: "list",
		type: "thread",
		query: $routeParams.query,
	};

	bbs.send(threadListCmd,
		$scope._list,
		$scope._error);
}

function LoginCtrl($scope, bbs, $location) {
	$scope.username = "";
	$scope.password = "";
	$scope.errorMsg = "";

	$scope._welcome = function(data) {
		bbs.user = data.username || $scope.username;
		bbs.session = data.session;
		$location.path(bbs.home);

		localStorage["sesh for " + bbs.addr] = data.session;
		localStorage["data for " + bbs.addr] = JSON.stringify(bbs.server);
		localStorage["last sesh"] = bbs.addr;
	}

	$scope._error = function(data) {
		$scope.errorMsg = data.error;
	}

	$scope.submit = function() {
		//todo: secure login

		var loginCmd = {
			cmd: "login",
			username: $scope.username,
			password: $scope.password,
			version: 0
		};

		bbs.send(loginCmd,
			$scope._welcome,
			$scope._error);
	}
}

function ThreadCtrl($scope, bbs, $location, $routeParams) {
	$scope.id = "";
	$scope.title = "";
	$scope.range = "";
	$scope.closed = false;
	$scope.filter = "";
	$scope.format = "";

	$scope.board = "";
	$scope.tags = null;

	$scope.messages = [];
	$scope.errorMsg = "";
	$scope.more = false;

	$scope._msg = function(data) {
		$scope.id = data.id;
		$scope.title = data.title || "";
		$scope.range = data.range || null;
		$scope.closed = data.closed || false;
		$scope.filter = data.filter || "";
		$scope.format = data.format || "";
		$scope.board = data.board || "";
		$scope.tags = data.tags || null;
		$scope.messages = data.messages;
	};

	$scope._error = function(data) {
		$scope.errorMsg = data.error;
	};

	var getCmd = {
		cmd: "get",
		id: $routeParams.id,
		format: "html",
	};

	if (bbs.connected()) {
		if (bbs.server.layout.range) {
			if (!$routeParams.range) {
				getCmd.range = bbs.server.defaultRange;
			} else {
				getCmd.range = bbs.parseRange($routeParams.range);
			}
		}
	}

	bbs.send(getCmd, $scope._msg, $scope._error);
}

function BoardCtrl($scope, bbs, $location) {
	$scope.boards = {}
	$scope.errorMsg = "";

	$scope._list = function(data) {
		$scope.boards = data.boards;
	}

	$scope._error = function(data) {
		$scope.errorMsg = data.error;
	}

	var boardListCmd = {
		cmd: "list",
		type: "board",
	};

	bbs.send(boardListCmd, $scope._list, $scope._error);
}

function ServerCtrl($scope, $http, $location, bbs) {
	$scope.servers = [
		{
			name: "localhost",
			addr: "/bbs"
		}
	];

	$scope.refresh = function() {
		_.each($scope.servers, function(s, i) {
			var cmd = {
				"cmd": "hello"
			};
			$http.post(s.addr, cmd).success(
				function(data) {
					$scope._hello(s, data);
				}
			);
		});
	}

	$scope._hello = function(server, cmd) {
		server.name = cmd.name;
		server.desc = cmd.desc;
		server.format = cmd.format;
		server.lists = cmd.lists;
		server.options = cmd.options;
		server.serverVersion = cmd.server;
		server.version = cmd.version;
		server.icon = cmd.icon;
		server.access = cmd.access;
		server.user = null;
		server.session = null;

		server.layout = {
			imageboard: false,
			avatars: false,
			titles: false,
			sigs: false,
			tags: false,
			range: false,
			requiresLogin: false,
		}

		//now we have to figure out how to lay out this board
		//first question: is it an image board? they are special
		if (_.contains(server.options, "imageboard")) {
			server.layout.imageboard = true;
		}
		//this client does not support imageboards with avatars etc
		else {
			if (_.contains(server.options, "avatars")) {
				server.layout.avatars = true;
			}
			if (_.contains(server.options, "usertitles")) {
				server.layout.usertitles = true;
			}
			if (_.contains(server.options, "signatures")) {
				server.layout.sigs = true;
			}
			if (_.contains(server.options, "tags")) {
				server.layout.tags = true;
			}
			if (_.contains(server.options, "range")) {
				server.layout.range = true;
				server.defaultRange = cmd.default_range;
			}
		}

		//next up we set up the home page
		//there will be better protocol stuff for this in the future
		if (_.contains(server.options, "boards") &&
			_.contains(server.lists, "board")) {
			server.home = "/boards";
		} else {
			server.home = "/threads/";
		}

		//finally, does the server need people to log in for them to do anything useful?
		if (_.contains(server.access.user, "list") ||
			_.contains(server.access.user, "get")) {
			if (localStorage["sesh for " + server.addr]) {
				server.session = localStorage["sesh for " + server.addr];
			} else {
				server.layout.requiresLogin = true;
			}
		}
	}

	$scope.connect = function(server) {
		console.log(bbs);
		console.log(server);
		bbs.server = server;
		bbs.addr = server.addr;
		bbs.session = server.session || null;
		bbs.home = server.home;

		//do we need to log in first?
		if (server.layout.requiresLogin) {
			$location.path("/login");
		} else {
			$location.path(server.home);
			localStorage["last sesh"] = server.addr;
			localStorage["data for " + server.addr] = JSON.stringify(bbs.server);
		}
		console.log("connect");
	}

	//setup
	$scope.refresh();
}