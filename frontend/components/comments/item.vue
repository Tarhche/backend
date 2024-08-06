<template>
  <section :class="{child:child }" v-if="data">
    <section class="card mb-3">
      <div class="card-body d-flex flex-start">
        <img v-if="data.author.avatar" class="rounded-circle shadow-1-strong ms-3"
             :src="useFilesUrlResolver().resolve(data.author.avatar)" alt="avatar" width="65"
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
              <button class="btn btn-sm" @click="showWriteComment">
                <i class="fas fa-reply fa-xs"></i>
              </button>
            </div>
          </div>
          <p v-if="data.body && data.body.length" class="text small mb-0 pt-2 border-top ">{{ data.body }}</p>
        </div>
      </div>
    </section>
    <section class="write-comment px-1" ref="writeComment">
      <comments-write-new :clear-data="clearDataAfterCloseComment" :disabled="disabled" :parentInfo="data.uuid"
                          @send-comment="sendComment" :replyTheme="true">
        <button class="btn btn-sm btn-md-lg btn-danger align-self-start mt-2" @click.prevent="showWriteComment">بستن
        </button>
      </comments-write-new>
    </section>
    <comments-item v-if="data.sub && data.sub.length" v-for="(item , index) in data.sub" :key="index" :data="item"
                   :child="true"/>
  </section>
</template>

<script setup lang="ts">
const {uuid} = useRoute().params
import {useFilesUrlResolver} from "~/composables/urlResolver";

const disabled = ref()
const writeComment = ref(null)
const clearDataAfterCloseComment = ref(false)
const {data, child} = defineProps({
  data: {
    type: Object,
  },
  child: {
    type: Boolean
  }
})
/* در فانکشن زیر ما برای ظاهر شدن کامپوننت کامنت از maxHeight استفاده کردیم
 به این صورت که در ابتدا صفر و با کلیک برروی ریپلای به اندازه طول
 اسکرول آن بعلاوه 20 پیکسل بیشتر که ارفاع ارور آن دررمان ظاهر شدن است */
const showWriteComment = () => {
  if (writeComment.value.style.maxHeight) {
    writeComment.value.style.maxHeight = null
    useState('clearDataAfterCloseComment').value = true
  } else {
    writeComment.value.style.maxHeight = writeComment.value.scrollHeight + 'px'
    useState('clearDataAfterCloseComment').value = false
  }

}
const sendComment = async (text: string, parentUuid: string) => {
  const body = {
    body: text,
    object_type: 'article',
    parent_uuid: parentUuid,
    object_uuid: uuid
  }
  try {
    disabled.value = true /* برای غیر فعال شدن دکمه ارسال کامنت بعد فشرده شدن تا زمان برشگت درخواست  */
    const data = await useUser().$fetch(useApiUrlResolver().resolve('api/comments'), {
      method: 'POST',
      headers: {authorization: `Bearer ${useAuth().accessToken()}`},
      body: JSON.stringify(body)
    })
    showWriteComment() /*برای بسته شدن ریپلای در صورت موفقیت آمیز بودن ارسال درخواست */
  } catch (e) {
    console.log(e)
  } finally {
    disabled.value = false /* عملیات ارسال دیتا در نهایت هرچی که باشه دکمه ما تغیر حالت میده  */
  }
}
const params = reactive({
  isLogin: useAuth().isLogin(),
})

</script>

<style scoped lang="scss">
card {
  transition: 0.5s;
}

.child {
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

.transform-enter-active {
  animation: reply ease-out 0.35s;
  z-index: 1;
}

.transform-leave-active {
  animation: reply ease-in 0.35s reverse;
}

.write-comment {
  max-height: 0;
  overflow: hidden;
  transition: 0.5s !important;
}

@keyframes reply {
  0% {
    transform: translateY(-10%);
    opacity: -1;
  }
  100% {
    transform: translateY(0);
    opacity: 1;
  }
}
</style>
