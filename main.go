package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	"encoding/json"
	"speech-recognition/audio"
	"speech-recognition/vad"

	vosk "github.com/alphacep/vosk-api/go"
)

const (
	sampleRate     = 16000
	framesPerBuffer = 1024
	calibrationTime = 5 * time.Second
)

func main() {
	// コマンドラインフラグ
	modelPath := flag.String("models", "./models/vosk-model-ja-0.22", "VOSKモデルのパス")
	vadMode := flag.Int("vad-mode", 1, "VAD感度 (0-3: 0=寛容、3=厳格)")
	calibrationDuration := flag.Duration("calibration-time", calibrationTime, "キャリブレーション時間")
	flag.Parse()

	fmt.Println("🎤 VOSKリアルタイム音声認識 起動中...")

	// VOSKモデルの読み込み
	fmt.Printf("モデル読み込み中: %s\n", *modelPath)
	model, err := vosk.NewModel(*modelPath)
	if err != nil {
		log.Fatalf("モデル読み込みエラー: %v", err)
	}
	defer model.Free()
	fmt.Println("✅ モデル読み込み完了")

	// 音声認識器の作成
	recognizer, err := vosk.NewRecognizer(model, float64(sampleRate))
	if err != nil {
		log.Fatalf("認識器作成エラー: %v", err)
	}
	defer recognizer.Free()

	// 部分結果を有効化
	recognizer.SetWords(1)

	// 適応的VADの初期化
	vadDetector := vad.NewAdaptiveVAD(sampleRate, *vadMode)

	// オーディオキャプチャの初期化
	capture, err := audio.NewCapture(sampleRate, framesPerBuffer)
	if err != nil {
		log.Fatalf("オーディオキャプチャエラー: %v", err)
	}
	defer capture.Close()

	fmt.Println("🎙️ マイクデバイス: デフォルト入力")
	fmt.Printf("⏱️ キャリブレーション中... (%v)\n", *calibrationDuration)

	// キャリブレーション期間
	if err := calibrate(capture, vadDetector, *calibrationDuration); err != nil {
		log.Fatalf("キャリブレーションエラー: %v", err)
	}

	threshold := vadDetector.GetThreshold()
	fmt.Printf("\n周囲ノイズレベル: %.3f (閾値: %.3f)\n", vadDetector.GetNoiseLevel(), threshold)
	fmt.Println("📡 音声認識開始\n")

	// シグナルハンドラ
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 音声認識ループ
	go recognitionLoop(capture, vadDetector, recognizer)

	// 終了待機
	<-sigChan
	fmt.Println("\n\n👋 終了します...")
}

func calibrate(capture *audio.Capture, vadDetector *vad.AdaptiveVAD, duration time.Duration) error {
	start := time.Now()
	for time.Since(start) < duration {
		buffer, err := capture.Read()
		if err != nil {
			return fmt.Errorf("キャリブレーション読み込みエラー: %w", err)
		}
		vadDetector.Calibrate(buffer)
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

func recognitionLoop(capture *audio.Capture, vadDetector *vad.AdaptiveVAD, recognizer *vosk.VoskRecognizer) {
	for {
		buffer, err := capture.Read()
		if err != nil {
			log.Printf("音声読み込みエラー: %v", err)
			continue
		}

		// 音圧レベル計算
		rms := vadDetector.CalculateRMS(buffer)

		// VAD判定
		isSpeech := vadDetector.IsSpeech(buffer)

		if isSpeech {
			// 音声区間 → VOSKに送信
			result := recognizer.AcceptWaveform(buffer)

			if result == 1 {
				// 確定結果
				finalResult := recognizer.Result()
				if len(finalResult) > 0 {
					fmt.Printf("\r\033[K[確定] %s\n", extractText(finalResult))
				}
			} else {
				// 部分結果
				partialResult := recognizer.PartialResult()
				if len(partialResult) > 0 {
					fmt.Printf("\r[部分] %s", extractText(partialResult))
				}
			}

			fmt.Printf("\r音圧: %.3f | 閾値: %.3f | 🔊 音声検出", rms, vadDetector.GetThreshold())
		} else {
			// 非音声区間 → 閾値更新
			vadDetector.UpdateThreshold(buffer)
		}
	}
}

func extractText(jsonResult string) string {
	       // JSONパースでtext/partialフィールドを抽出
	       var result struct {
		       Text    string `json:"text"`
		       Partial string `json:"partial"`
	       }
	       if err := json.Unmarshal([]byte(jsonResult), &result); err != nil {
		       return ""
	       }
	       if result.Text != "" {
		       return result.Text
	       }
	       if result.Partial != "" {
		       return result.Partial
	       }
	       return ""
}
