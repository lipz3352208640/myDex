--  CREATE TABLE `block`
--  (
--      `id`           bigint                                                         NOT NULL AUTO_INCREMENT,
--      `slot`         bigint                                                         NOT NULL DEFAULT '0' COMMENT 'slot',
--      `block_height` bigint                                                         NOT NULL DEFAULT '0' COMMENT 'block_height',
--      `block_time`   timestamp                                                      NOT NULL COMMENT 'block_time',
--      `status`       tinyint                                                        NOT NULL DEFAULT '0' COMMENT '1 processed, 2 failed',
--      `sol_price`    decimal(64, 18)                                                NOT NULL DEFAULT '0.000000000000000000' COMMENT 'sol price',
--      `created_at`   timestamp                                                      NOT NULL DEFAULT CURRENT_TIMESTAMP,
--      `updated_at`   timestamp                                                      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
--      `deleted_at`   timestamp                                                      NULL     DEFAULT NULL,
--      `err_message`  varchar(1000) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'error message',
--      PRIMARY KEY (`id`),
--      UNIQUE KEY `slot_index` (`slot`),
--      KEY `block_time_index` (`block_time`)
--  ) ENGINE = InnoDB
--    DEFAULT CHARSET = utf8mb4
--    COLLATE = utf8mb4_general_ci COMMENT ='block';



-- CREATE TABLE `token` (
--   `id` bigint NOT NULL AUTO_INCREMENT,
--   `chain_id` int NOT NULL DEFAULT '1' COMMENT 'Chain ID',--链id
--   `address` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'Token contract address',--代币地址
--   `program` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'program',--代币所在合约
--   `name` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT 'Token name',--代币名称
--   `symbol` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'Token symbol',--代币符号
--   `decimals` tinyint(1) NOT NULL DEFAULT '18' COMMENT 'Token decimals',--代币精度
--   `total_supply` double NOT NULL DEFAULT '0' COMMENT 'Total token supply',--代币总发行量
--   `icon` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT 'Token icon URL',--代币图标url
--   `description` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT 'Token description',--代币描述
--   `hold_count` int NOT NULL DEFAULT '0' COMMENT 'Number of holders',--持有者数量
--   `is_ca_drop_owner` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'Owner rights renounced',--是否放弃owner权限
--   `is_ca_verify` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'Contract verified',--合约是否已验证
--   `is_honey_scam` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'Honeypot check (Cannot sell)',--是否蜜罐
--   `is_liquid_lock` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'Liquidity locked',--流动性是否锁定
--   `is_can_pause_trade` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'Can pause trading',--是否可暂停交易
--   `is_can_change_tax` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'Can modify tax rate',--是否可修改税率
--   `is_have_black_list` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'Has blacklist mechanism',--是否有黑名单机制
--   `is_can_all_sell` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'Can sell entire balance',--是否允许全部卖出
--   `is_have_proxy` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'Has proxy contract',--是否是代理合约
--   `is_can_external_call` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'Contract can make external calls',--是否可外部调用
--   `is_can_add_token` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'Contract has minting capability',--是否可增大
--   `is_can_change_token` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'Owner can modify user balances',--是否可修改用户余额
--   `sell_tax` decimal(10,4) NOT NULL DEFAULT '0.0000' COMMENT 'Sell tax rate',--卖出税
--   `buy_tax` decimal(10,4) NOT NULL DEFAULT '0.0000' COMMENT 'Buy tax rate',--买入税
--   `twitter_username` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT 'Twitter username',--推特账号
--   `website` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT 'Official website',--官网
--   `telegram` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT 'Telegram link',--电报群链接
--   `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
--   `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
--   `deleted_at` timestamp NULL DEFAULT NULL,
--   `is_check_ca` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'Contract analysis completed',--是否已完成合约安全分析
--   `check_ca_at` bigint NOT NULL DEFAULT '0' COMMENT 'Contract analysis timestamp',--完成分析的时间戳
--   `is_burn_pool` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'Indicates if the token is part of a burn pool',--是否属于燃烧池相关代币
--   `is_top_ten` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'Indicates if the token is in the top ten by market capitalization',--是否进入前十市值
--   `audit_source` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'Audit source',--审计来源
--   `slot` bigint NOT NULL DEFAULT '0',
--   PRIMARY KEY (`id`) USING BTREE,
--   UNIQUE KEY `chain_id_address_index` (`chain_id`,`address`) USING BTREE,
--   UNIQUE KEY `chain_id_address_symbol_index` (`chain_id`,`address`,`symbol`) USING BTREE,
--   KEY `address_index` (`address`) USING BTREE,
--   KEY `idx_created_audit` (`created_at` DESC) USING BTREE,
--   KEY `icon_index` (`icon`(255)) USING BTREE
-- ) ENGINE=InnoDB AUTO_INCREMENT=1165832 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci ROW_FORMAT=DYNAMIC COMMENT='Token Table';





CREATE TABLE `pair` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `chain_id` int NOT NULL DEFAULT '1' COMMENT 'Chain ID',
  `address` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'Trading pair address',--pair地址
  `name` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT 'DEX factory version swap name',--交易对所属版本/来源名，比如某个 DEX 或 swap 名称
  `factory_address` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'Factory contract address',--工厂合约地址
  `base_token_address` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'Base token address',--基础币地址
  `token_address` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'Token address',--代币地址
  `base_token_symbol` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'Base token symbol',--基础代币符号
  `token_symbol` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'Token symbol',--token符号
  `base_token_decimal` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'Base token decimals',--基础代币精度
  `token_decimal` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'Token decimals',--代币精度
  `base_token_is_native_token` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'Is base token native currency',--基础代币是否原生代币
  `base_token_is_token0` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'Is base token token0',--是否对应pair中的token0
  `init_base_token_amount` decimal(64,18) NOT NULL DEFAULT '0.000000000000000000' COMMENT 'Initial base token liquidity',--池子初始化base token 数量 
  `init_token_amount` decimal(64,18) NOT NULL DEFAULT '0.000000000000000000' COMMENT 'Initial token liquidity',--池子初始化token 数量 
  `current_base_token_amount` decimal(64,18) NOT NULL DEFAULT '0.000000000000000000' COMMENT 'Current base token liquidity',--当前池子base token流动性
  `current_token_amount` decimal(64,18) NOT NULL DEFAULT '0.000000000000000000' COMMENT 'Current token liquidity',--当前池子token流动性
  `fdv` double NOT NULL DEFAULT '0' COMMENT 'Fully diluted valuation',--完全稀释估值
  `mkt_cap` decimal(64,18) NOT NULL DEFAULT '0.000000000000000000' COMMENT 'Market capitalization',--市值
  `token_price` decimal(64,18) NOT NULL DEFAULT '0.000000000000000000' COMMENT 'Token price',--当前token价格
  `base_token_price` decimal(64,18) NOT NULL DEFAULT '0.000000000000000000' COMMENT 'Base token price',--当前base token价格
  `block_num` int NOT NULL DEFAULT '0' COMMENT 'Creation block height',--区块号
  `block_time` timestamp NOT NULL COMMENT 'Creation block timestamp',--出块时间
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `deleted_at` timestamp NULL DEFAULT NULL,
  `highest_token_price` decimal(64,18) NOT NULL DEFAULT '0.000000000000000000' COMMENT 'Highest token price',--历史最高 token 价格
  `latest_trade_time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'Latest on-chain trade timestamp',--最近一次链上交易时间
  `pump_point` decimal(64,18) NOT NULL DEFAULT '0.000000000000000000' COMMENT 'Pump score',--pump 分数/进度类指标
  `launch_pad_point` double DEFAULT '0' COMMENT 'LaunchPad progress',--launchpad 进度
  `pump_launched` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'Pump launched (0: false, 1: true)',--是否已 launch
  `pump_market_cap` decimal(64,18) NOT NULL DEFAULT '0.000000000000000000' COMMENT 'Pump market cap',--pump 市值
  `pump_owner` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'Pump owner address',--pump owner 地址
  `pump_swap_pair_addr` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'Pump swap pair address',--迁移/发射后对应 swap pair 地址
  `pump_virtual_base_token_reserves` decimal(64,18) NOT NULL DEFAULT '0.000000000000000000' COMMENT 'Pump virtual base token reserves',--pump 虚拟 base token 储备
  `pump_virtual_token_reserves` decimal(64,18) NOT NULL DEFAULT '0.000000000000000000' COMMENT 'Pump virtual token reserves',--pump 虚拟 token 储备
  `pump_status` tinyint NOT NULL DEFAULT '0' COMMENT 'Pump status',--pump 状态
  `pump_pair_addr` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'Pump pair address',--pump 相关 pair 地址
  `slot` bigint NOT NULL DEFAULT '0',
  `liquidity` decimal(64,18) NOT NULL DEFAULT '0.000000000000000000' COMMENT 'Liquidity',--总流动性
  `launch_pad_status` int NOT NULL DEFAULT '0' COMMENT 'LaunchPad status: 0-not launchpad, 1-new creation, 2-completing, 3-completed'--launchpad 状态,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `chain_id_address_index` (`chain_id`,`address`) USING BTREE,
  KEY `name_index` (`name`) USING BTREE,
  KEY `token_address_index` (`token_address`) USING BTREE,
  KEY `token_symbol_index` (`token_symbol`) USING BTREE,
  KEY `pump_point_index` (`pump_point`) USING BTREE,
  KEY `block_num_index` (`block_num`) USING BTREE,
  KEY `pump_status_index` (`pump_status`) USING BTREE,
  KEY `block_time_index` (`block_time`) USING BTREE
) ENGINE=InnoDB AUTO_INCREMENT=1099042 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci ROW_FORMAT=DYNAMIC COMMENT='Pair Table';



-- CREATE TABLE `trade` (
--   `id` bigint NOT NULL AUTO_INCREMENT,
--   `chain_id` int NOT NULL DEFAULT '0' COMMENT 'Chain ID',
--   `pair_addr` varchar(64) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'Trading pair contract address',
--   `tx_hash` varchar(256) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'Transaction hash',
--   `hash_id` varchar(256) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'Transaction ID',
--   `maker` varchar(64) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'Transaction initiator',
--   `trade_type` varchar(64) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'Trade type',
--   `base_token_amount` decimal(64,18) NOT NULL DEFAULT '0.000000000000000000' COMMENT 'Base token amount in this trade',
--   `token_amount` decimal(64,18) NOT NULL DEFAULT '0.000000000000000000' COMMENT 'Token amount in this trade',
--   `base_token_price_usd` decimal(64,18) NOT NULL DEFAULT '0.000000000000000000' COMMENT 'Base token price in USD',
--   `total_usd` decimal(64,18) NOT NULL DEFAULT '0.000000000000000000' COMMENT 'Total trade value in USD',
--   `token_price_usd` decimal(64,18) NOT NULL DEFAULT '0.000000000000000000' COMMENT 'Token price in USD',
--   `to` varchar(64) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'Token recipient',
--   `block_num` bigint NOT NULL DEFAULT '0' COMMENT 'Block height',
--   `block_time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'Block timestamp',
--   `block_time_stamp` bigint NOT NULL DEFAULT '0' COMMENT 'Transaction timestamp',
--   `swap_name` varchar(64) COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'DEX name',
--   `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
--   `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
--   `deleted_at` timestamp NULL DEFAULT NULL,
--   PRIMARY KEY (`id`),
--   UNIQUE KEY `hash_id_index` (`hash_id`),
--   KEY `marker_index` (`maker`),
--   KEY `block_time_index` (`block_time`)
-- ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='Trade records';
