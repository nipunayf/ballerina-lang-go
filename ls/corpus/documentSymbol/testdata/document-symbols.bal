@deprecated
function old() {}

type Person record {|
    string name;
    int age = 0;
    string...;
|};

class User {
    string id;
    function init() {}
    function greet() {}
}

enum Color {
    RED,
    BLUE
}

const int LIMIT = 10;
string emoji = "😀";
annotation string Label on function;
xmlns "urn:example" as ex;

service /foo {
    resource function get users() returns string {
        return "";
    }
}
