# Pubkemail Client

- Website: [https://pubkemail.com](https://pubkemail.com/)
- Current Version: 0.1.0
- Discourse Discussion Boards: (https://discuss.pubkemail.com)
- **As of Pubkemail-Client 2018.07-15.0.1.0, Go 1.7+ is required to build from source**

The Pubkemail Client is used to retrieve and forward emails that were sent to a BTC address using the `@pubkemail.com` domain.

for example, you can send email to:`1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa@pubkemail.com`

The Client is expected to run on your machine in the background so that emails can be forwarded as they come in. **Note:** you will need to have access to a SMTP server or HTTP API to forward emails. Places that provide SMTP and/or HTTP API services can be found by searching the internet for something like: `"send email api"` 


## Quick Start


[![GoDoc](http://img.shields.io/badge/godoc-reference-5272B4.svg?style=plastic)](https://godoc.org/github.com/pubkemail/client)

First, [download a pre-built Pubkemail Client binary](https://github.com/pubkemail/client/releases) for your operating system or compile a Client yourself.

After the Client is installed, you can run it by double clicking on the downloaded binary it will open to a terminal view. Within the terminal view there is a link to the Web Interface. Browse using your favorite web browswer to that location. This is where you can interact with the terminal application through a more user friendly interface.

Inside of the web interface add your SMTP or HTTP-API JSON. This is how you will forward emails.

```json
{
    "http-api": {
        "v1": {
            "method":"POST",
            "url": "https://api.example.net/v3/sandbox.example.org/messages",
            "user":"api",
            "pass":"example-api-password",
            "parameters": {
                "from":["Example <example@sandbox.example.org>"],
                "to": ["user@example.com"],
                "subject": "{{ .Subject }}",
                "text": "{{ .Text }}",
                "html": "{{ .HTML }}"
            },
            "headers": {
                "content-type": "multipart/form-data"
            }
        }
    }
}
```

Next, add the WIFs of address that you would like to check.

The Client will check the pubkemail RRS feeds for new emails based on the addresses of the WIFs you have supplied. When an email is found it will be forwarded to your email. The RSS feed goes back for 3 months unless you have a plan.

### Compiling a Client

```bash
git clone git@github.com:pubkemail/client.git pubkemail-client
go generate -run "prd_lin"
```

We support  `go generate -run "<env>_<os>"` for **amd64** Linux, Windows and macOS environments. Otherwise use the `GOOS`, `GOARCH` and build instructions found in the offical go documentation. https://golang.org/cmd/go/#hdr-Build_and_test_caching

## Contributing

[![Build Status](https://semaphoreci.com/api/v1/pubkemail-client/client/branches/master/shields_badge.svg)](https://semaphoreci.com/pubkemail-client/client)

The Pubkemail Client is **open source**. We encourage and support an active, healthy community that accepts public contributions!

Before contributing to the Pubkemail Client:

1. Read [**CONTRIBUTING.md**](https://github.com/pubkemail/client/blob/master/.github/CONTRIBUTING.md), which covers submitting bugs, requesting new features, preparing your code for a pull request, etc.
2. We have a list things to work on. (http://discuss.pubkemail.com/help-wanted/3823)

We look forward to seeing what you want to contribute! 

## Versioning

[![GitHub release](https://img.shields.io/github/release/pubkemail/client.svg?style=plastic)](https://img.shields.io/github/release/pubkemail/client.svg?style=plastic)

We use [SemVer](http://semver.org/) as the basis of your versioning scheme, however we still have a few extra ideas around versioning.

* The rules of how to bump major, minor and patch numbers should be strict and done by a program (with as little human intervention as possible) ` *Not Implemented* `
* The bump should happen on every build even dev builds ` *Implemented* `
* There should be a universal build number that flows across all builds (even including the website and internal service changes) ` *Not Implemented* `
* production builds can be prefixed with a `<year>.<month>-<day>` i.e (`2018.07-15.0.1.0`) ` *Implemented* `

## Authors

- **Nika Jones** - *Initial work* - [Pubkemail](https://github.com/njones)

See also the list of [contributors](https://github.com/pubkemail/client/CONTRIBUTORS) who participated in develop, extend and maintain this client.

## Acknowledgments

- The Go team
- SMTP implementors (but definitely not spammers)
- RSS Creators
- Bitcoin/Blockchain Users

## License

Copyright (c) 2018 Pubkemail

Licensed under the GNU General Public License v3; you may not use this work except in compliance with the License. You may obtain a copy of the License in the LICENSE file, or at:

<https://www.gnu.org/licenses/gpl.txt>

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions and limitations under the License.