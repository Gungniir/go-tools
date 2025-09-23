Live Templates For jetbrains

```xml
<template name="collgen" value="//go:generate go tool github.com/gungniir/go-tools/collgen -file $FILE$.go -out $FILE$_gen.go -marker collgen:gen" description="" toReformat="false" toShortenFQNames="true">
    <variable name="FILE" expression="fileNameWithoutExtension()" defaultValue="" alwaysStopAt="false" />
    <context>
        <option name="GO_FILE" value="true" />
    </context>
</template>
<template name="collgenstruct" value="//collgen:gen" description="" toReformat="false" toShortenFQNames="true">
<context>
    <option name="GO_FILE" value="true" />
</context>
</template>
<template name="dddagg" value="type $name$ struct {&#10;&#9;domain.BaseAggregate[$lowername$events.Type]&#10;&#10;&#9;ID           $lowername$values.ID&#10;&#9;Status       $lowername$values.Status&#10;&#9;$END$&#10;}&#10;&#10;func NewDefault(id $lowername$values.ID) *$name$ {&#10;&#9;return &amp;$name${&#10;&#9;&#9;ID:      id,&#10;&#9;&#9;Status:  $lowername$values.StatusDraft,&#10;&#9;}&#10;}" description="" toReformat="false" toShortenFQNames="true">
<variable name="name" expression="" defaultValue="" alwaysStopAt="true" />
<variable name="lowername" expression="camelCase(name)" defaultValue="" alwaysStopAt="false" />
<context>
    <option name="GO_FILE" value="true" />
</context>
</template>
<template name="dddevent" value="type $Event$ struct {&#10;&#9;domain.BaseEvent[Type]&#10;&#10;&#9;$END$&#10;}&#10;&#10;func (e *$Event$) Type() Type {&#10;&#9;return Type$Event$&#10;}" description="" toReformat="false" toShortenFQNames="true">
<variable name="Event" expression="" defaultValue="" alwaysStopAt="true" />
<context>
    <option name="GO_FILE" value="true" />
</context>
</template>
<template name="dddid" value="type ID uuid.UUID&#10;&#10;func GenerateID() ID {&#10;&#9;return ID(uuid.New())&#10;}&#10;&#10;func ParseID(s string) (ID, error) {&#10;&#9;id, err := uuid.Parse(s)&#10;&#9;return ID(id), domain.ErrInconsistent.Wrap(err)&#10;}&#10;&#10;func (id ID) String() string {&#10;&#9;return uuid.UUID(id).String()&#10;}&#10;&#10;func (id ID) MarshalText() ([]byte, error) {&#10;&#9;return []byte(id.String()), nil&#10;}&#10;&#10;func (id *ID) UnmarshalText(text []byte) (err error) {&#10;&#9;*id, err = ParseID(string(text))&#10;&#9;return err&#10;}" description="" toReformat="false" toShortenFQNames="true">
<context>
    <option name="GO_FILE" value="true" />
</context>
</template>
<template name="enumgen" value="//go:generate go tool github.com/gungniir/go-tools/enumgen -file $FILE$.go -out $FILE$_gen.go -marker enumgen:gen" description="" toReformat="false" toShortenFQNames="true">
<variable name="FILE" expression="fileNameWithoutExtension()" defaultValue="" alwaysStopAt="false" />
<context>
    <option name="GO_FILE" value="true" />
</context>
</template>
<template name="enumgenstruct" value="//enumgen:gen" description="" toReformat="false" toShortenFQNames="true">
<context>
    <option name="GO_FILE" value="true" />
</context>
</template>
```