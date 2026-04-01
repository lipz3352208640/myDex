package slot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"myDex/myConsumer/internal/logic/entity"
	"myDex/myConsumer/internal/svc"
	"net"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/proc"
	"github.com/zeromicro/go-zero/core/threading"
)

type SlotService struct {
	ctx            *svc.ServiceContext
	conn           *websocket.Conn
	subscriptionID int64
	//日志
	logx.Logger
	//服务名
	name string
	//服务当前上下文
	context context.Context
	//服务取消
	cancle   func(err error)
	slotChan chan uint64
	//
}

func NewSlotService(sc *svc.ServiceContext, slotChan chan uint64, name string) *SlotService {
	ctx, cancle := context.WithCancelCause(context.Background())
	return &SlotService{
		ctx:      sc,
		Logger:   logx.WithContext(ctx).WithFields(logx.Field("service", name)),
		context:  ctx,
		cancle:   cancle,
		slotChan: slotChan,
	}
}

func (s *SlotService) Start() {
	proc.AddShutdownListener(func() {
		s.Info("slot get success")
		s.cancle(errors.New("slot get success"))
	})

	threading.GoSafe(func() {

		for {
			select {
			case <-s.context.Done():
				s.Info("acquire slot stop success")
				return
			default:
			}
			slot := s.getSlot()
			s.slotChan <- slot
		}
	})
}

func (s *SlotService) getSlot() uint64 {
	s.ConnectWs()
	//重试读取订阅消息
	for {
		_, message, err := s.conn.ReadMessage()
		if err != nil {
			s.Error("err is %v", err.Error())
			var ne net.Error
			switch {
			case errors.As(err, &ne) && (ne.Timeout() || ne.Temporary()):
				s.resetConn()
				s.ConnectWs()
				continue
			case errors.Is(err, io.EOF):
				s.resetConn()
				s.ConnectWs()
				continue
			default:
				errMsg := strings.ToLower(err.Error())
				if strings.Contains(errMsg, "timeout") ||
					strings.Contains(errMsg, "close") ||
					strings.Contains(errMsg, "broken pipe") {
					s.resetConn()
					s.ConnectWs()
					continue
				}
			}
			return 0
		}
		//s.Infof("helius msg is %s\n", string(message))

		//fmt.Printf("helius msg is %s\n", string(message))

		var errResp entity.WsErrResp
		if err := json.Unmarshal(message, &errResp); err == nil && errResp.Error.Message != "" {
			s.Errorf("receive websocket error response, code=%d message=%s data=%s",
				errResp.Error.Code, errResp.Error.Message, errResp.Error.Data)
			continue
		}

		var resp entity.WsResp
		if err := json.Unmarshal(message, &resp); err != nil {
			s.Errorf("receive websocket message unmarshal fail, err is %v", err)
			continue
		}

		if resp.Method != "slotNotification" {
			continue
		}

		return resp.Params.Result.Slot
	}
}

// 获取websocket连接
func (s *SlotService) ConnectWs() {
	if s.conn != nil {
		return
	}

	//创建ws客户端拨号
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 5 * time.Second
	//创建客户端,重试
	for {
		conn, _, err := dialer.Dial(s.ctx.Config.Helius.WSUrl, nil)
		if err != nil {
			s.Errorf("sendWebSocketConnErr is %v", err)
		} else {
			var wsSub entity.WsSub
			subMsg := wsSub.ApplyWsSub()
			err := conn.WriteMessage(websocket.TextMessage, subMsg)
			var i int
			for i < 10 {
				if err != nil {
					s.Errorf("slot subscription err is %v", err)
				} else {
					//拿到确认ack消息
					_, message, err := conn.ReadMessage()
					if err != nil {
						s.Errorf("read slot subscription ack err is %v", err)
						break
					}

					//s.Infof("helius subscribe ack is %s\n", string(message))

					var errResp entity.WsErrResp
					if err := json.Unmarshal(message, &errResp); err == nil && errResp.Error.Message != "" {
						s.Errorf("slot subscription response err, code=%d message=%s data=%s",
							errResp.Error.Code, errResp.Error.Message, errResp.Error.Data)
						break
					}

					var ack entity.WsSubAck
					if err := json.Unmarshal(message, &ack); err != nil {
						s.Errorf("slot subscription ack unmarshal err is %v", err)
						break
					}
					if ack.Result == 0 {
						s.Errorf("slot subscription ack missing subscription id")
						break
					}

					s.conn = conn
					s.subscriptionID = ack.Result
					return
				}
				i++
			}
			conn.Close()
		}
		time.Sleep(time.Second)
	}
}

func (s *SlotService) resetConn() {
	if s.conn != nil {
		s.conn.Close()
	}
	s.conn = nil
	s.subscriptionID = 0
}

func (s *SlotService) Stop() {
	s.Info("slot service close")
	s.cancle(errors.New("myConsumer service stop"))

	//发送取消订阅消息
	if s.conn != nil && s.subscriptionID != 0 {
		var wsSub entity.WsUnsub
		unsubMessage := wsSub.CancleWsSub(s.subscriptionID)

		err := s.conn.WriteMessage(websocket.TextMessage, unsubMessage)
		if err != nil {
			s.Errorf("cancleSubcriptionErr %v", err)
		}
	}
	s.resetConn()
}
