<script setup lang="ts">
import img from '~/assets/images/bg-title-02.jpg'
const data = ref({
  cardName:'chat',
  userName:'john smith',
  you:{
    time:"12 Min ago",
    firstMessage:"Lorem ipsum dolor sit amet elit",
    secondMessage:"Donec tempor viverra",
  },
  me:{
    time:"30 Sec ago",
    firstMessage:"Lorem ipsum dolor sit amet elit"
  },
  placeHolder:"پیام خود را بنویسید"
})
</script>

<template>
  <div class="au-card au-card--no-shadow au-card--no-pad m-b-40 au-card--border">
    <div class="au-card-title" :style="{background:`url(${img}) center/cover`}">
      <div class="bg-overlay bg-overlay--blue"></div>
      <h3>
        <i class="zmdi zmdi-comment-text"></i>{{data.cardName}}</h3>
      <button class="au-btn-plus">
        <i class="zmdi zmdi-plus"></i>
      </button>
    </div>
    <div class="au-inbox-wrap">
      <div class="au-chat au-chat--border">
        <div class="au-chat__title ">
          <div class="au-chat-info  flex-row-reverse">
            <div class="avatar-wrap online">
              <div class="avatar avatar--small">
                <img src="~/assets/images/icon/avatar-02.jpg" alt="John Smith">
              </div>
            </div>
            <span class="nick">
              <nuxt-link class="text-decoration-none" to="#">{{data.userName}}</nuxt-link>
            </span>
          </div>
        </div>
        <div class="au-chat__content au-chat__content2 js-scrollbar5">
          <div class="recei-mess-wrap">
            <span class="mess-time">{{data.you.time}}</span>
            <div class="recei-mess__inner">
              <div class="avatar avatar--tiny">
                <img src="~/assets/images/icon/avatar-02.jpg" alt="John Smith">
              </div>
              <div class="recei-mess-list">
                <div class="recei-mess">{{data.you.firstMessage}}</div>
                <div class="recei-mess">{{data.you.secondMessage}}</div>
              </div>
            </div>
          </div>
          <div class="send-mess-wrap">
            <span class="mess-time">{{data.me.time}}</span>
            <div class="send-mess__inner">
              <div class="send-mess-list">
                <div class="send-mess">{{data.me.firstMessage}}</div>
              </div>
            </div>
          </div>
        </div>
        <div class="au-chat-textfield">
          <form class="au-form-icon  ">
            <input class="au-input au-input--full au-input--h65 " type="text"
                   :placeholder="data.placeHolder">
            <button class="au-input-icon">
              <i class="fa-solid fa-camera"></i>
            </button>
          </form>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>

</style>