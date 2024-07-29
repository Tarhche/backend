<template>
  <section :class="{child:child }" v-if="data">
    <section class="card mb-3">
    <div class="card-body d-flex flex-start">
      <img v-if="data.author.avatar" class="rounded-circle shadow-1-strong ms-3" :src="useFilesUrlResolver().resolve(data.author.avatar)" alt="avatar" width="65"
           height="65">
      <div class="flex-grow-1 flex-shrink-1 ">
        <div class="d-flex justify-content-between align-items-center">
          <p class="info mb-1">
            <span v-if="data.author.name">{{ data.author.name }}</span>
            <span class="text-muted small me-1" v-if="data.created_at && data.created_at.length">
              <span class="fa-regular fa-clock mx-1"></span>
              <time datetime="">{{ useTime().toAgo(data.created_at) }}</time>
            </span>
          </p>
          <div v-if="params.isLogin" class="text-nowrap">
            <button class="btn text-danger btn-sm" v-if="false">
              <i class="fas fa-trash fa-xs"></i>
            </button>
            <span v-if="false">|</span>
            <button class="btn btn-sm">
              <i class="fas fa-reply fa-xs"></i>
            </button>
          </div>
        </div>
        <p v-if="data.body && data.body.length" class="text small mb-0 pt-2 border-top ">{{ data.body }}</p>
      </div>
    </div>
  </section>
    <comments-item v-if="data.sub && data.sub.length" v-for="(item , index) in data.sub" :key="index" :data="item" :child="true" />
  </section>

</template>

<script setup lang="ts">
import {useFilesUrlResolver} from "~/composables/urlResolver";

const {data , child} = defineProps({
  data: {
    type: Object,
  },
  child:{
    type:Boolean
  }
})
const params = reactive({
  isLogin: useAuth().isLogin(),
})
</script>

<style scoped lang="scss">
.child{
  margin-right: 3%;
}
p {
  &.text {
    line-height: 30px;
    font-size: 14px;
    color: #6c757d;
  }

  &.info {
    line-height: 20px;
    font-size: 14px;
  }
}
</style>
