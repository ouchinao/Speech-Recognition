package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	"github.com/gordonklaus/portaudio"
)

const (
	sampleRate          = 16000
	channelCount        = 1
	framesPerBuffer     = 1024
	calibrationDuration = 3 * time.Second
	minAudioDuration    = 400 * time.Millisecond
	silenceDuration     = 500 * time.Millisecond
)

type AudioProcessor struct {
	model          whisper.Model
	context        whisper.Context
	stream         *portaudio.Stream
	inputBuffer    []int16
	audioBuffer    []float32
	noiseThreshold float32
	isCalibrated   bool
}

// モデル読み込みとコンテキスト作成
func NewAudioProcessor(modelPath string) (*AudioProcessor, error) {
	model, err := whisper.New(modelPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load model: %v", err)
	}

	ctx, err := model.NewContext()
	if err != nil {
		model.Close()
		return nil, fmt.Errorf("failed to create context: %v", err)
	}

	ctx.SetLanguage("ja")

	ap := &AudioProcessor{
		model:       model,
		context:     ctx,
		inputBuffer: make([]int16, framesPerBuffer),
		audioBuffer: make([]float32, 0),
	}
	return ap, nil
}

func (ap *AudioProcessor) Close() {
	if ap.stream != nil {
		ap.stream.Close()
	}
	if ap.model != nil {
		ap.model.Close()
	}
}

// RMS音圧計算（int16用）
func calculateRMSInt16(samples []int16) float32 {
	if len(samples) == 0 {
		return 0
	}
	var sumSquares float64
	for _, sample := range samples {
		normalized := float64(sample) / 32768.0
		sumSquares += normalized * normalized
	}
	rms := math.Sqrt(sumSquares / float64(len(samples)))
	return float32(rms)
}

// ストリームを初期化
func (ap *AudioProcessor) InitStream() error {
	if ap.stream != nil {
		return nil
	}

	if err := portaudio.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize PortAudio: %v", err)
	}

	stream, err := portaudio.OpenDefaultStream(
		channelCount,
		0,
		float64(sampleRate),
		framesPerBuffer,
		ap.inputBuffer,
	)
	if err != nil {
		portaudio.Terminate()
		return fmt.Errorf("failed to open audio stream: %v", err)
	}
	ap.stream = stream

	if err := ap.stream.Start(); err != nil {
		ap.stream.Close()
		portaudio.Terminate()
		return fmt.Errorf("failed to start audio stream: %v", err)
	}

	fmt.Println("✅ PortAudioストリーム初期化完了")
	return nil
}

// ノイズ閾値のキャリブレーション
func (ap *AudioProcessor) CalibrateNoiseThreshold() error {
	ap.isCalibrated = true
	defer func() {
		ap.isCalibrated = false
	}()

	fmt.Println("🔊 環境ノイズ計測中（3秒間）...")
	fmt.Println("   静かにしてお待ちください...\n")
	
	if err := ap.InitStream(); err != nil {
		return err
	}

	var rmsValues []float32
	startTime := time.Now()
	
	fmt.Println("🎤 マイクテスト中...")
	for time.Since(startTime) < calibrationDuration {
		if err := ap.stream.Read(); err != nil {
			return fmt.Errorf("failed to read from audio stream: %v", err)
		}
		
		rms := calculateRMSInt16(ap.inputBuffer)
		rmsValues = append(rmsValues, rms)
		
		fmt.Printf("   サンプル RMS: %.6f\n", rms)
		
		time.Sleep(100 * time.Millisecond)
	}

	var maxRMS float32
	var avgRMS float32
	for _, rms := range rmsValues {
		if rms > maxRMS {
			maxRMS = rms
		}
		avgRMS += rms
	}
	if len(rmsValues) > 0 {
		avgRMS /= float32(len(rmsValues))
	}

	// 閾値設定：平均値の3倍または最大値の50%、どちらか大きい方
	threshold1 := avgRMS * 3.0
	threshold2 := maxRMS * 0.5
	
	if threshold1 > threshold2 {
		ap.noiseThreshold = threshold1
	} else {
		ap.noiseThreshold = threshold2
	}
	
	// 最低閾値を設定
	if ap.noiseThreshold < 0.005 {
		ap.noiseThreshold = 0.005
	}

	fmt.Printf("\n✅ キャリブレーション完了\n")
	fmt.Printf("   平均ノイズレベル: %.6f\n", avgRMS)
	fmt.Printf("   最大ノイズレベル: %.6f\n", maxRMS)
	fmt.Printf("   音声検出閾値: %.6f\n\n", ap.noiseThreshold)
	
	return nil
}

// 音声認識開始
func (ap *AudioProcessor) StartRecognition() error {
	fmt.Println("🎤 音声認識開始（Ctrl+Cで終了）")
	fmt.Println("   話しかけてください...\n")

	lastSpeechTime := time.Now()
	isSpeaking := false
	frameCount := 0
	silenceFrameCount := 0

	for {
		if err := ap.stream.Read(); err != nil {
			return fmt.Errorf("failed to read from audio stream: %v", err)
		}

		rms := calculateRMSInt16(ap.inputBuffer)
		frameCount++

		// 定期的に音圧レベルを表示（5秒ごと）
		if frameCount%50 == 0 {
			fmt.Printf("🔉 音圧: %.6f (閾値: %.6f)\n", rms, ap.noiseThreshold)
		}

		// 音声検出
		if rms > ap.noiseThreshold {
			silenceFrameCount = 0
			
			if !isSpeaking {
				fmt.Printf("\n🗣️  音声検出！ (RMS: %.6f)\n", rms)
				isSpeaking = true
			}

			// int16をfloat32に変換
			for _, sample := range ap.inputBuffer {
				ap.audioBuffer = append(ap.audioBuffer, float32(sample)/32768.0)
			}

			lastSpeechTime = time.Now()
			
			// 1秒ごとに録音時間を表示
			if len(ap.audioBuffer)%(sampleRate*1) == 0 {
				audioLength := float64(len(ap.audioBuffer)) / float64(sampleRate)
				fmt.Printf("   録音中: %.1f秒\n", audioLength)
			}
		} else {
			// 無音状態
			if isSpeaking {
				silenceFrameCount++
				
				// 一定時間無音が続いたら認識実行
				if time.Since(lastSpeechTime) > silenceDuration {
					audioLength := float64(len(ap.audioBuffer)) / float64(sampleRate)
					
					fmt.Printf("\n📝 音声認識開始... (%.1f秒分)\n", audioLength)
					
					// 最低0.4秒の音声が必要
					minSamples := int(float64(sampleRate) * minAudioDuration.Seconds())
					if len(ap.audioBuffer) > minSamples {
						if err := ap.ProcessAudio(); err != nil {
							fmt.Printf("❌ 認識エラー: %v\n\n", err)
						}
					} else {
						fmt.Printf("⚠️  音声が短すぎます (%.1f秒 < 0.4秒)\n\n", audioLength)
					}
					
					ap.audioBuffer = make([]float32, 0)
					isSpeaking = false
					silenceFrameCount = 0
					fmt.Println("📢 待機中...\n")
				}
			}
		}
	}
}

// Whisper処理
func (ap *AudioProcessor) ProcessAudio() error {
	if len(ap.audioBuffer) == 0 {
		return fmt.Errorf("audio buffer is empty")
	}

	startTime := time.Now()

	// Whisperで音声認識を実行
	if err := ap.context.Process(ap.audioBuffer, nil, nil, nil); err != nil {
		return fmt.Errorf("failed to process audio: %v", err)
	}

	// 認識結果を取得
	hasResult := false
	
	for {
		segment, err := ap.context.NextSegment()
		if err != nil {
			break
		}
		
		if segment.Text != "" {
			hasResult = true
			fmt.Printf("📄 結果: %s\n", segment.Text)
		}
	}

	processingTime := time.Since(startTime)

	if !hasResult {
		fmt.Println("⚠️  認識結果なし（雑音または不明瞭な音声）")
	}
	
	fmt.Printf("⏱️  処理時間: %.2f秒\n", processingTime.Seconds())
	fmt.Println("==========================================\n")

	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("使用方法: go run main.go <Whisperモデルパス>")
		fmt.Println("例: go run main.go ./models/ggml-small.bin")
		os.Exit(1)
	}
	modelPath := os.Args[1]

	processor, err := NewAudioProcessor(modelPath)
	if err != nil {
		log.Fatalf("Error initializing audio processor: %v", err)
	}
	defer processor.Close()
	defer portaudio.Terminate()

	if err := processor.CalibrateNoiseThreshold(); err != nil {
		log.Fatalf("Error during calibration: %v", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	errChan := make(chan error, 1)
	go func() {
		errChan <- processor.StartRecognition()
	}()

	select {
	case <-sigChan:
		fmt.Println("\n\n✅ プログラム終了")
	case err := <-errChan:
		if err != nil {
			log.Fatalf("Error during recognition: %v", err)
		}
	}
}
