package options

import (
	"encoding/json"
	"testing"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestLoggerConfigValidate(t *testing.T) {
	t.Run("NoType", func(t *testing.T) {
		config := LoggerConfig{
			info: loggerConfigInfo{Format: RawLoggerConfigFormatJSON},
		}
		assert.Error(t, config.validate())
	})
	t.Run("InvalidLoggerConfigFormat", func(t *testing.T) {
		config := LoggerConfig{
			info: loggerConfigInfo{
				Type:   LogDefault,
				Format: "foo",
				Config: []byte("some bytes"),
			},
		}
		assert.Error(t, config.validate())
	})
	t.Run("UnsetRegistry", func(t *testing.T) {
		config := LoggerConfig{
			info: loggerConfigInfo{
				Type:   LogDefault,
				Format: RawLoggerConfigFormatBSON,
			},
		}
		assert.NoError(t, config.validate())
		assert.Equal(t, globalLoggerRegistry, config.Registry)
	})
	t.Run("SetRegistry", func(t *testing.T) {
		registry := NewBasicLoggerRegistry()
		config := LoggerConfig{
			Registry: registry,
			info: loggerConfigInfo{
				Type:   LogDefault,
				Format: RawLoggerConfigFormatBSON,
			},
		}
		assert.NoError(t, config.validate())
		assert.Equal(t, registry, config.Registry)
	})
}

func TestLoggerConfigSet(t *testing.T) {
	t.Run("UnregisteredLogger", func(t *testing.T) {
		config := LoggerConfig{
			Registry: NewBasicLoggerRegistry(),
			info: loggerConfigInfo{
				Format: RawLoggerConfigFormatBSON,
			},
		}
		assert.Error(t, config.Set(&DefaultLoggerOptions{}))
		assert.Empty(t, config.info.Type)
		assert.Nil(t, config.producer)
	})
	t.Run("RegisteredLogger", func(t *testing.T) {
		config := LoggerConfig{
			Registry: globalLoggerRegistry,
			info: loggerConfigInfo{
				Format: RawLoggerConfigFormatBSON,
			},
		}
		require.NoError(t, config.Set(&DefaultLoggerOptions{}))
		assert.Equal(t, LogDefault, config.info.Type)
		assert.Equal(t, &DefaultLoggerOptions{}, config.producer)
	})
}

func TestLoggerConfigResolve(t *testing.T) {
	t.Run("InvalidConfig", func(t *testing.T) {
		config := LoggerConfig{}
		require.Error(t, config.validate())
		sender, err := config.Resolve()
		assert.Nil(t, sender)
		assert.Error(t, err)
	})
	t.Run("UnregisteredLogger", func(t *testing.T) {
		config := LoggerConfig{
			Registry: NewBasicLoggerRegistry(),
			info: loggerConfigInfo{
				Type:   LogDefault,
				Format: RawLoggerConfigFormatBSON,
			},
		}
		require.NoError(t, config.validate())
		sender, err := config.Resolve()
		assert.Nil(t, sender)
		assert.Error(t, err)
	})
	t.Run("MismatchingFormatAndConfig", func(t *testing.T) {
		rawData, err := json.Marshal(&DefaultLoggerOptions{Prefix: "prefix"})
		require.NoError(t, err)
		config := LoggerConfig{
			Registry: globalLoggerRegistry,
			info: loggerConfigInfo{
				Type:   LogDefault,
				Format: RawLoggerConfigFormatBSON,
				Config: rawData,
			},
		}
		require.NoError(t, config.validate())
		require.True(t, config.Registry.Check(config.info.Type))
		sender, err := config.Resolve()
		assert.Nil(t, sender)
		assert.Error(t, err)
	})
	t.Run("MismatchingConfigAndProducer", func(t *testing.T) {
		rawData, err := json.Marshal(&DefaultLoggerOptions{Prefix: "prefix"})
		require.NoError(t, err)
		config := LoggerConfig{
			Registry: globalLoggerRegistry,
			info: loggerConfigInfo{
				Type:   LogFile,
				Format: RawLoggerConfigFormatJSON,
				Config: rawData,
			},
		}
		require.NoError(t, config.validate())
		require.True(t, config.Registry.Check(config.info.Type))
		sender, err := config.Resolve()
		assert.Nil(t, sender)
		assert.Error(t, err)
	})
	t.Run("InvalidProducerConfig", func(t *testing.T) {
		config := LoggerConfig{
			Registry: globalLoggerRegistry,
			info: loggerConfigInfo{
				Type:   LogFile,
				Format: RawLoggerConfigFormatBSON,
			},
			producer: &FileLoggerOptions{},
		}
		require.NoError(t, config.validate())
		require.True(t, config.Registry.Check(config.info.Type))
		sender, err := config.Resolve()
		assert.Nil(t, sender)
		assert.Error(t, err)
	})
	t.Run("SenderUnset", func(t *testing.T) {
		config := LoggerConfig{
			Registry: globalLoggerRegistry,
			info: loggerConfigInfo{
				Type:   LogDefault,
				Format: RawLoggerConfigFormatBSON,
			},
			producer: &DefaultLoggerOptions{Base: BaseOptions{Format: LogFormatPlain}},
		}
		sender, err := config.Resolve()
		assert.NotNil(t, sender)
		assert.NoError(t, err)
	})
	t.Run("ProducerAndSenderUnsetJSON", func(t *testing.T) {
		rawConfig, err := json.Marshal(&DefaultLoggerOptions{Base: BaseOptions{Format: LogFormatPlain}})
		require.NoError(t, err)
		config := LoggerConfig{
			Registry: globalLoggerRegistry,
			info: loggerConfigInfo{
				Type:   LogDefault,
				Format: RawLoggerConfigFormatJSON,
				Config: rawConfig,
			},
		}
		sender, err := config.Resolve()
		assert.NotNil(t, sender)
		assert.NoError(t, err)
	})
	t.Run("ProducerAndSenderUnsetBSON", func(t *testing.T) {
		rawConfig, err := bson.Marshal(&DefaultLoggerOptions{Base: BaseOptions{Format: LogFormatPlain}})
		require.NoError(t, err)
		config := LoggerConfig{
			Registry: globalLoggerRegistry,
			info: loggerConfigInfo{
				Type:   LogDefault,
				Format: RawLoggerConfigFormatBSON,
				Config: rawConfig,
			},
		}
		sender, err := config.Resolve()
		assert.NotNil(t, sender)
		assert.NoError(t, err)
	})
}

func TestLoggerConfigMarshalBSON(t *testing.T) {
	t.Run("InvalidConfig", func(t *testing.T) {
		config := LoggerConfig{
			info: loggerConfigInfo{
				Type:   LogDefault,
				Config: []byte("some bytes"),
			},
		}
		_, err := bson.Marshal(&config)
		assert.Error(t, err)
	})
	t.Run("UnregisteredLogger", func(t *testing.T) {
		config := LoggerConfig{
			Registry: NewBasicLoggerRegistry(),
			info: loggerConfigInfo{
				Type:   LogDefault,
				Format: RawLoggerConfigFormatBSON,
				Config: []byte("some bytes"),
			},
		}
		_, err := bson.Marshal(&config)
		assert.Error(t, err)
	})
	t.Run("JSON2BSON", func(t *testing.T) {
		producer := &DefaultLoggerOptions{
			Prefix: "jasper",
			Base: BaseOptions{
				Level: send.LevelInfo{
					Default:   level.Info,
					Threshold: level.Info,
				},
				Format: LogFormatPlain,
			},
		}
		rawConfig, err := json.Marshal(producer)
		require.NoError(t, err)
		config := LoggerConfig{
			info: loggerConfigInfo{
				Type:   LogDefault,
				Format: RawLoggerConfigFormatJSON,
				Config: rawConfig,
			},
		}
		data, err := bson.Marshal(&config)
		require.NoError(t, err)
		assert.NotNil(t, data)
		unmarshalledConfig := &LoggerConfig{}
		require.NoError(t, bson.Unmarshal(data, unmarshalledConfig))
		assert.Equal(t, config.info.Type, unmarshalledConfig.info.Type)
		assert.Equal(t, RawLoggerConfigFormatBSON, unmarshalledConfig.info.Format)
		_, err = config.Resolve()
		require.NoError(t, err)
		assert.Equal(t, producer, config.producer)
	})
	t.Run("ExistingProducer", func(t *testing.T) {
		config := LoggerConfig{
			info: loggerConfigInfo{
				Type:   LogDefault,
				Format: RawLoggerConfigFormatBSON,
				Config: []byte("some bytes"),
			},
			producer: &DefaultLoggerOptions{
				Prefix: "jasper",
				Base: BaseOptions{
					Level: send.LevelInfo{
						Default:   level.Info,
						Threshold: level.Info,
					},
					Format: LogFormatPlain,
				},
			},
		}
		data, err := bson.Marshal(&config)
		require.NoError(t, err)
		assert.NotNil(t, data)
		unmarshalledConfig := &LoggerConfig{}
		require.NoError(t, bson.Unmarshal(data, unmarshalledConfig))
		assert.Equal(t, config.info.Type, unmarshalledConfig.info.Type)
		assert.Equal(t, RawLoggerConfigFormatBSON, unmarshalledConfig.info.Format)
		_, err = unmarshalledConfig.Resolve()
		require.NoError(t, err)
		assert.Equal(t, config.producer, unmarshalledConfig.producer)
	})
	t.Run("RoundTrip", func(t *testing.T) {
		rawConfig, err := bson.Marshal(&DefaultLoggerOptions{
			Prefix: "jasper",
			Base: BaseOptions{
				Level: send.LevelInfo{
					Default:   level.Info,
					Threshold: level.Info,
				},
				Format: LogFormatPlain,
			},
		})
		require.NoError(t, err)
		config := LoggerConfig{
			Registry: globalLoggerRegistry,
			info: loggerConfigInfo{
				Type:   LogDefault,
				Format: RawLoggerConfigFormatBSON,
				Config: rawConfig,
			},
		}
		data, err := bson.Marshal(&config)
		require.NoError(t, err)
		roundTripped := &LoggerConfig{}
		require.NoError(t, bson.Unmarshal(data, roundTripped))
		sender, err := roundTripped.Resolve()
		assert.NotNil(t, sender)
		assert.NoError(t, err)
		assert.Equal(t, config.info.Config, roundTripped.info.Config)
	})
}

func TestLoggerConfigMarshalJSON(t *testing.T) {
	t.Run("InvalidConfig", func(t *testing.T) {
		config := LoggerConfig{
			info: loggerConfigInfo{
				Type:   LogDefault,
				Config: []byte("some bytes"),
			},
		}
		_, err := json.Marshal(&config)
		assert.Error(t, err)
	})
	t.Run("UnregisteredLogger", func(t *testing.T) {
		config := LoggerConfig{
			Registry: NewBasicLoggerRegistry(),
			info: loggerConfigInfo{
				Type:   LogDefault,
				Format: RawLoggerConfigFormatJSON,
				Config: []byte("some bytes"),
			},
		}
		_, err := json.Marshal(&config)
		assert.Error(t, err)
	})
	t.Run("BSON2JSON", func(t *testing.T) {
		producer := &DefaultLoggerOptions{
			Prefix: "jasper",
			Base: BaseOptions{
				Level: send.LevelInfo{
					Default:   level.Info,
					Threshold: level.Info,
				},
				Format: LogFormatPlain,
			},
		}
		rawConfig, err := bson.Marshal(producer)
		require.NoError(t, err)
		config := LoggerConfig{
			info: loggerConfigInfo{
				Type:   LogDefault,
				Format: RawLoggerConfigFormatBSON,
				Config: rawConfig,
			},
		}
		data, err := json.Marshal(&config)
		require.NoError(t, err)
		assert.NotNil(t, data)
		unmarshalledConfig := &LoggerConfig{}
		require.NoError(t, json.Unmarshal(data, unmarshalledConfig))
		assert.Equal(t, config.info.Type, unmarshalledConfig.info.Type)
		assert.Equal(t, RawLoggerConfigFormatJSON, unmarshalledConfig.info.Format)
		_, err = config.Resolve()
		require.NoError(t, err)
		assert.Equal(t, producer, config.producer)
	})
	t.Run("ExistingProducer", func(t *testing.T) {
		config := LoggerConfig{
			info: loggerConfigInfo{
				Type:   LogDefault,
				Format: RawLoggerConfigFormatJSON,
				Config: []byte("some bytes"),
			},
			producer: &DefaultLoggerOptions{
				Prefix: "jasper",
				Base: BaseOptions{
					Level: send.LevelInfo{
						Default:   level.Info,
						Threshold: level.Info,
					},
					Format: LogFormatPlain,
				},
			},
		}
		data, err := json.Marshal(&config)
		require.NoError(t, err)
		assert.NotNil(t, data)
		unmarshalledConfig := &LoggerConfig{}
		require.NoError(t, json.Unmarshal(data, unmarshalledConfig))
		assert.Equal(t, config.info.Type, unmarshalledConfig.info.Type)
		assert.Equal(t, RawLoggerConfigFormatJSON, unmarshalledConfig.info.Format)
		_, err = unmarshalledConfig.Resolve()
		require.NoError(t, err)
		assert.Equal(t, config.producer, unmarshalledConfig.producer)
	})
	t.Run("RoundTrip", func(t *testing.T) {
		rawConfig, err := json.Marshal(&DefaultLoggerOptions{
			Prefix: "jasper",
			Base: BaseOptions{
				Level: send.LevelInfo{
					Default:   level.Info,
					Threshold: level.Info,
				},
				Format: LogFormatPlain,
			},
		})
		require.NoError(t, err)
		config := LoggerConfig{
			Registry: globalLoggerRegistry,
			info: loggerConfigInfo{
				Type:   LogDefault,
				Format: RawLoggerConfigFormatJSON,
				Config: rawConfig,
			},
		}
		data, err := json.Marshal(&config)
		require.NoError(t, err)
		roundTripped := &LoggerConfig{}
		require.NoError(t, json.Unmarshal(data, roundTripped))
		sender, err := roundTripped.Resolve()
		assert.NotNil(t, sender)
		assert.NoError(t, err)
		assert.Equal(t, config.info.Config, roundTripped.info.Config)
	})
}
