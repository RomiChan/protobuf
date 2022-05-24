# protobuf

This package is design for [Mrs4s/MiraiGo](https://github.com/Mrs4s/MiraiGo), 
aims to provide a lightweight runtime, so it only contains basic Marshal and 
Unmarshal API. Obviously, it does not support descriptors or reflection.

## Highlights

- Less generated code
- Less binary size after compiled
- Less heap alloc when using proto2
