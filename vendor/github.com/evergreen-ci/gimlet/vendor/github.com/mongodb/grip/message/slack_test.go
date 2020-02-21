package message

import (
	"reflect"
	"testing"

	"github.com/bluele/slack"
	"github.com/stretchr/testify/assert"
)

func TestSlackAttachmentFieldConvert(t *testing.T) {
	assert := assert.New(t) //nolint

	gripField := SlackAttachmentField{
		Title: "1",
		Value: "2",
		Short: true,
	}
	slackField := gripField.convert()

	assert.Equal("1", slackField.Title)
	assert.Equal("2", slackField.Value)
	assert.True(slackField.Short)
}

func TestSlackAttachmentConvert(t *testing.T) {
	assert := assert.New(t) //nolint

	af := SlackAttachmentField{
		Title: "1",
		Value: "2",
		Short: true,
	}

	at := SlackAttachment{
		Color:      "1",
		Fallback:   "2",
		AuthorName: "3",
		AuthorIcon: "6",
		Title:      "7",
		TitleLink:  "8",
		Text:       "10",
		Fields:     []*SlackAttachmentField{&af},
		MarkdownIn: []string{"15", "16"},
	}
	slackAttachment := at.convert()

	assert.Equal("1", slackAttachment.Color)
	assert.Equal("2", slackAttachment.Fallback)
	assert.Equal("3", slackAttachment.AuthorName)
	assert.Equal("6", slackAttachment.AuthorIcon)
	assert.Equal("7", slackAttachment.Title)
	assert.Equal("8", slackAttachment.TitleLink)
	assert.Equal("10", slackAttachment.Text)
	assert.Equal([]string{"15", "16"}, slackAttachment.MarkdownIn)
	assert.Len(slackAttachment.Fields, 1)
	assert.Equal("1", slackAttachment.Fields[0].Title)
	assert.Equal("2", slackAttachment.Fields[0].Value)
	assert.True(slackAttachment.Fields[0].Short)
}

func TestSlackAttachmentIsSame(t *testing.T) {
	assert := assert.New(t) //nolint

	grip := SlackAttachment{}
	slack := slack.Attachment{}

	vGrip := reflect.TypeOf(grip)
	vSlack := reflect.TypeOf(slack)

	for i := 0; i < vSlack.NumField(); i++ {
		slackField := vSlack.Field(i)
		gripField, found := vGrip.FieldByName(slackField.Name)
		if !found {
			continue
		}

		referenceTag := slackField.Tag.Get("json")
		assert.NotEmpty(referenceTag)
		jsonTag := gripField.Tag.Get("json")
		assert.Equal(referenceTag, jsonTag, "SlackAttachment.%s should have json tag with value: \"%s\"", gripField.Name, referenceTag)
		bsonTag := gripField.Tag.Get("bson")
		assert.Equal(referenceTag, bsonTag, "SlackAttachment.%s should have bson tag with value: \"%s\"", gripField.Name, referenceTag)
		yamlTag := gripField.Tag.Get("yaml")
		assert.Equal(referenceTag, yamlTag, "SlackAttachment.%s should have yaml tag with value: \"%s\"", gripField.Name, referenceTag)
	}

}

func TestSlackAttachmentFieldIsSame(t *testing.T) {
	assert := assert.New(t) //nolint

	gripStruct := SlackAttachmentField{}
	slackStruct := slack.AttachmentField{}

	vGrip := reflect.TypeOf(gripStruct)
	vSlack := reflect.TypeOf(slackStruct)

	assert.Equal(vSlack.NumField(), vGrip.NumField())
	for i := 0; i < vSlack.NumField(); i++ {
		slackField := vSlack.Field(i)
		gripField, found := vGrip.FieldByName(slackField.Name)
		assert.True(found, "field %s found in slack.AttachmentField, but not in message.SlackAttachmentField", slackField.Name)
		if !found {
			continue
		}

		referenceTag := slackField.Tag.Get("json")
		assert.NotEmpty(referenceTag)
		jsonTag := gripField.Tag.Get("json")
		assert.Equal(referenceTag, jsonTag, "SlackAttachmentField.%s should have json tag with value: \"%s\"", gripField.Name, referenceTag)
		bsonTag := gripField.Tag.Get("bson")
		assert.Equal(referenceTag, bsonTag, "SlackAttachmentField.%s should have bson tag with value: \"%s\"", gripField.Name, referenceTag)
		yamlTag := gripField.Tag.Get("yaml")
		assert.Equal(referenceTag, yamlTag, "SlackAttachmentField.%s should have yaml tag with value: \"%s\"", gripField.Name, referenceTag)

		assert.Equal(slackField.Type.Kind(), gripField.Type.Kind())
	}
}
