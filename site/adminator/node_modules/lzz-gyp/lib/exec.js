'use strict';
var which = require('which');
var spawn = require('child_process').spawn;
var spawnOptions = {
	encoding: 'utf8',
	stdio: 'inherit',
	shell: true
};

module.exports = function (file, args, cwd) {
	return new Promise(function (resolve, reject) {
		console.log('==>', 'cwd:', cwd);
		console.log('==>', [file].concat(args).join(' '));

        // Resolve the file to an absolute path to make sure it works
        // on windows. Otherwise if you run a command with quotes, the
        // shell resolves it to the wrong directory.
        file = which.sync(file);

		var child = spawn('"' + file + '"', args, Object.assign({cwd: cwd}, spawnOptions));
		child.on('exit', function (code) {
			if (code === 0) {
				resolve();
			} else {
				reject(new Error('exit code ' + code));
			}
		});
	});
};
