<script setup lang="ts">
const {parentInfo, replyTheme, disabled} = defineProps({
  parentInfo: {
    type: String
  },
  replyTheme: {
    type: Boolean
  },
  disabled: {
    type: Boolean
  }
})
const emit = defineEmits(['sendComment'])
const commentData = reactive({
  body: "",
  error: false
})
watch(() => useState('clearDataAfterCloseComment').value, () => {
  commentData.body = ""
})

const sendComment = () => {
  if (commentData.body.length > 4) {
    emit('sendComment', commentData.body, parentInfo)
  } else {
    commentData.error = true
  }
}
const removeError = () => {
  commentData.error ? commentData.error = false : ""
}
</script>

<template>
  <form class="my-3 py-1 d-flex flex-column" :class="{'mt-0':replyTheme}" @submit.prevent="sendComment">
    <textarea class="form-control mb-1" placeholder="دیدگاه خود را اینجا بنویسید" rows="3" required
              v-model.trim="commentData.body" @keyup="removeError"></textarea>
    <span class="error text-danger" v-if="commentData.error">متن پیام باید بیشتر باشد .</span>
    <div class="d-flex gap-2">
      <button :class="`btn  btn-success align-self-start ${ replyTheme ?'btn-sm mt-2': 'mt-3'}`" :disabled="disabled"
              type="submit">ثبت دیدگاه
      </button>
      <slot/>
    </div>
  </form>
</template>

<style scoped>
.error {
  font-size: 12px;
}
</style>
